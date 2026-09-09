package sender

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"go.uber.org/zap"
	"gorm.io/datatypes"
)

// WebhookSender 自定义 Webhook 消息发送器。
//
// 面向"标题 + 内容"两类消息的通用 HTTP 推送渠道：
//   - 请求体支持 {{title}} / {{content}} / {{timestamp}} 占位符模板，
//     值按 content_type 自动转义（JSON 内引号/换行安全，form 内 URL 转义）
//   - 支持 POST/PUT/PATCH（带请求体）与 GET（占位符拼入查询串）
//   - 支持自定义请求头（如 Authorization）
type WebhookSender struct {
	logger *zap.Logger
}

// NewWebhookSender 创建 Webhook 发送器实例
func NewWebhookSender(logger *zap.Logger) *WebhookSender {
	return &WebhookSender{logger: logger}
}

type webhookConfig struct {
	URL          string         `json:"url"`           // 目标地址（必填）
	Method       string         `json:"method"`        // POST/PUT/PATCH/GET，默认 POST
	ContentType  string         `json:"content_type"`  // application/json(默认)/application/x-www-form-urlencoded/text/plain
	Headers      webhookHeaders `json:"headers"`       // 自定义请求头：JSON 对象或 "Key: Value" 每行一个
	BodyTemplate string         `json:"body_template"` // 请求体模板，空则按 content_type 使用默认模板
	TimeoutSec   int            `json:"timeout_sec"`   // 请求超时秒数，默认 30，范围 1-120
}

// webhookHeaders 灵活解析请求头配置：
//  1. JSON 对象：{"Authorization":"Bearer xxx","X-Source":"octotify"}
//  2. 字符串（每行一个，冒号支持中英文）：
//     Authorization: Bearer xxx
//     X-Source: octotify
type webhookHeaders map[string]string

func (h *webhookHeaders) UnmarshalJSON(b []byte) error {
	trimmed := strings.TrimSpace(string(b))
	*h = webhookHeaders{}
	if trimmed == "" || trimmed == "null" {
		return nil
	}
	// 形式 1：JSON 对象
	if trimmed[0] == '{' {
		var obj map[string]string
		if err := json.Unmarshal(b, &obj); err != nil {
			return fmt.Errorf("headers 应为 JSON 对象或每行一个 \"Key: Value\": %w", err)
		}
		for k, v := range obj {
			(*h)[strings.TrimSpace(k)] = strings.TrimSpace(v)
		}
		return nil
	}
	// 形式 2：字符串，按行解析 "Key: Value"
	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		return fmt.Errorf("headers 应为 JSON 对象或每行一个 \"Key: Value\": %w", err)
	}
	for _, line := range strings.Split(s, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		sep := strings.IndexAny(line, ":\uFF1A") // 半角/全角冒号
		if sep <= 0 {
			return fmt.Errorf("请求头行格式错误（应为 Key: Value）: %s", line)
		}
		key := strings.TrimSpace(line[:sep])
		val := strings.TrimSpace(line[sep+utf8len(line[sep]):])
		(*h)[key] = val
	}
	return nil
}

// utf8len 返回 UTF-8 编码下首字节 rune 的字节长度（用于跳过全角冒号等多字节字符）
func utf8len(b byte) int {
	switch {
	case b < 0x80:
		return 1
	case b < 0xE0:
		return 2
	case b < 0xF0:
		return 3
	default:
		return 4
	}
}

type webhookTemplateData struct {
	Title     string
	Content   string
	Timestamp int64 // Unix 秒
}

const (
	webhookHTTPTimeout = 30 * time.Second
	webhookRespLimit   = 64 << 10 // 响应体最多读取 64KB（仅用于错误诊断）
)

var webhookAllowedMethods = map[string]bool{
	http.MethodPost: true, http.MethodPut: true, http.MethodPatch: true, http.MethodGet: true,
}

func (s *WebhookSender) Send(ctx context.Context, config datatypes.JSON, title string, content string) error {
	var cfg webhookConfig
	if err := json.Unmarshal(config, &cfg); err != nil {
		return fmt.Errorf("解析 Webhook 渠道配置失败: %w", err)
	}

	// ---- 配置规范化与校验 ----
	if strings.TrimSpace(cfg.URL) == "" {
		return fmt.Errorf("Webhook 目标地址不能为空")
	}
	method := strings.ToUpper(strings.TrimSpace(cfg.Method))
	if method == "" {
		method = http.MethodPost
	}
	if !webhookAllowedMethods[method] {
		return fmt.Errorf("不支持的请求方法: %s（仅 POST/PUT/PATCH/GET）", cfg.Method)
	}
	contentType := strings.TrimSpace(cfg.ContentType)
	if contentType == "" {
		contentType = "application/json"
	}
	if contentType != "application/json" && contentType != "application/x-www-form-urlencoded" && contentType != "text/plain" {
		return fmt.Errorf("不支持的 Content-Type: %s（仅 application/json / application/x-www-form-urlencoded / text/plain）", cfg.ContentType)
	}
	timeout := webhookHTTPTimeout
	if cfg.TimeoutSec > 0 {
		if cfg.TimeoutSec > 120 {
			cfg.TimeoutSec = 120
		}
		timeout = time.Duration(cfg.TimeoutSec) * time.Second
	}

	// 目标地址校验（允许 http/https；不强制 https 以兼容内网自建服务）
	target := strings.TrimSpace(cfg.URL)
	if !strings.HasPrefix(target, "http://") && !strings.HasPrefix(target, "https://") {
		target = "https://" + target
	}
	parsedURL, err := url.Parse(target)
	if err != nil || parsedURL.Host == "" {
		return fmt.Errorf("Webhook 目标地址格式错误")
	}

	data := webhookTemplateData{
		Title:     title,
		Content:   content,
		Timestamp: time.Now().Unix(),
	}

	// ---- 组装请求 ----
	var req *http.Request
	if method == http.MethodGet {
		// GET：占位符以查询串传递，忽略请求体
		q := parsedURL.Query()
		q.Set("title", data.Title)
		q.Set("content", data.Content)
		parsedURL.RawQuery = q.Encode()
		req, err = http.NewRequestWithContext(ctx, method, parsedURL.String(), nil)
	} else {
		body, ct, berr := webhookRenderBody(cfg, contentType, data)
		if berr != nil {
			return berr
		}
		req, err = http.NewRequestWithContext(ctx, method, parsedURL.String(), bytes.NewReader(body))
		if err == nil {
			// 用户未在自定义头里指定 Content-Type 时使用模板对应的默认值
			if _, exists := cfg.Headers["Content-Type"]; !exists {
				req.Header.Set("Content-Type", ct)
			}
		}
	}
	if err != nil {
		return fmt.Errorf("创建 HTTP 请求失败: %w", err)
	}

	// 自定义请求头
	for k, v := range cfg.Headers {
		if strings.EqualFold(k, "Host") { // Host 不可通过 Header 设置
			continue
		}
		req.Header.Set(k, v)
	}

	// 日志脱敏：仅记录 host 与自定义头名称，不记录完整 URL（可能内嵌 token）与头的值
	headerNames := make([]string, 0, len(cfg.Headers))
	for k := range cfg.Headers {
		headerNames = append(headerNames, k)
	}
	s.logger.Debug("Webhook 渠道配置",
		zap.String("host", parsedURL.Host),
		zap.String("method", method),
		zap.Strings("custom_headers", headerNames),
	)

	client := &http.Client{Timeout: timeout}
	resp, err := client.Do(req)
	if err != nil {
		s.logger.Error("Webhook 网络请求失败",
			zap.String("host", parsedURL.Host),
			zap.Error(err),
		)
		return fmt.Errorf("发送 Webhook 请求失败: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, webhookRespLimit))
	if err != nil {
		return fmt.Errorf("读取 Webhook 响应失败: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		s.logger.Error("Webhook 返回非 2xx 响应",
			zap.String("host", parsedURL.Host),
			zap.Int("http_status", resp.StatusCode),
			zap.ByteString("body_snippet", respBody),
		)
		return fmt.Errorf("Webhook 返回 HTTP 错误: %d, body: %s", resp.StatusCode, truncateMessage(string(respBody), 512, "\n[响应已截断]"))
	}

	s.logger.Info("Webhook 推送成功",
		zap.String("host", parsedURL.Host),
		zap.Int("http_status", resp.StatusCode),
	)
	return nil
}

// webhookRenderBody 按内容类型渲染请求体。
// 返回 (请求体字节, Content-Type, 错误)。
func webhookRenderBody(cfg webhookConfig, contentType string, data webhookTemplateData) ([]byte, string, error) {
	switch contentType {
	case "application/json":
		tpl := cfg.BodyTemplate
		if strings.TrimSpace(tpl) == "" {
			tpl = `{"title":"{{title}}","content":"{{content}}"}`
		}
		// JSON 模式：占位符位于字符串字面量内，仅做 JSON 内层转义（引号由模板提供）
		body := webhookReplace(tpl,
			webhookJSONInner(data.Title),
			webhookJSONInner(data.Content),
			strconv.FormatInt(data.Timestamp, 10),
		)
		if !json.Valid([]byte(body)) {
			return nil, "", fmt.Errorf("渲染后的请求体不是合法 JSON，请检查 body_template（提示：占位符值会自动转义，但需自行保证模板结构合法）")
		}
		return []byte(body), "application/json", nil

	case "application/x-www-form-urlencoded":
		tpl := cfg.BodyTemplate
		if strings.TrimSpace(tpl) == "" {
			tpl = `title={{title}}&content={{content}}`
		}
		body := webhookReplace(tpl, url.QueryEscape(data.Title), url.QueryEscape(data.Content), strconv.FormatInt(data.Timestamp, 10))
		return []byte(body), "application/x-www-form-urlencoded", nil

	default: // text/plain
		tpl := cfg.BodyTemplate
		if strings.TrimSpace(tpl) == "" {
			tpl = "{{title}}\n\n{{content}}"
		}
		body := webhookReplace(tpl, data.Title, data.Content, strconv.FormatInt(data.Timestamp, 10))
		return []byte(body), "text/plain", nil
	}
}

// webhookReplace 单趟替换全部占位符（避免 content 内含 {{title}} 字面量导致二次替换）
func webhookReplace(tpl, titleEscaped, contentEscaped, timestamp string) string {
	return strings.NewReplacer(
		"{{content}}", contentEscaped,
		"{{title}}", titleEscaped,
		"{{timestamp}}", timestamp,
		"{{ title }}", titleEscaped,
		"{{ content }}", contentEscaped,
		"{{ timestamp }}", timestamp,
	).Replace(tpl)
}

// webhookJSONInner 产出 JSON 字符串的内层转义文本（不含两侧引号）。
// 模板中写作 "key":"{{title}}"，引号由模板提供。
func webhookJSONInner(s string) string {
	b, err := json.Marshal(s)
	if err != nil {
		return ""
	}
	out := string(b)
	// json.Marshal 结果恒为 "..." 形式
	if len(out) >= 2 && out[0] == '"' && out[len(out)-1] == '"' {
		out = out[1 : len(out)-1]
	}
	return out
}
