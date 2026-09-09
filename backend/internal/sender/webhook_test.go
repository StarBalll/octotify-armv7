package sender

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"go.uber.org/zap/zaptest"
	"gorm.io/datatypes"
)

// TestWebhookSender_NewInstance 测试Webhook发送器新建实例持有logger
func TestWebhookSender_NewInstance(t *testing.T) {
	logger := zaptest.NewLogger(t)
	sender := NewWebhookSender(logger)

	if sender == nil {
		t.Fatal("NewWebhookSender() returned nil")
	}
	if sender.logger == nil {
		t.Error("NewWebhookSender() sender.logger is nil")
	}
}

func TestWebhookSender_Send(t *testing.T) {
	tests := []struct {
		name        string
		config      datatypes.JSON
		title       string
		content     string
		handler     http.HandlerFunc
		wantErr     bool
		errContains string
		verify      func(t *testing.T, r *http.Request, body string)
	}{
		{
			name:    "成功_默认JSON模板",
			config:  datatypes.JSON(`{"url":"http://example.com/hook"}`),
			title:   "构建通知",
			content: "第一行\n第二行 \"引号\" 与 \\ 反斜杠",
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
			},
			verify: func(t *testing.T, r *http.Request, body string) {
				if r.Method != http.MethodPost {
					t.Errorf("方法应为 POST，got %s", r.Method)
				}
				if ct := r.Header.Get("Content-Type"); ct != "application/json" {
					t.Errorf("Content-Type 应为 application/json，got %s", ct)
				}
				if !json.Valid([]byte(body)) {
					t.Fatalf("请求体应为合法 JSON: %s", body)
				}
				var m map[string]any
				if err := json.Unmarshal([]byte(body), &m); err != nil {
					t.Fatalf("解析请求体失败: %v", err)
				}
				if m["title"] != "构建通知" {
					t.Errorf("title 不匹配: %v", m["title"])
				}
				want := "第一行\n第二行 \"引号\" 与 \\ 反斜杠"
				if m["content"] != want {
					t.Errorf("content 应转义后完整还原:\n want %q\n got  %q", want, m["content"])
				}
			},
		},
		{
			name: "成功_自定义模板与请求头",
			config: datatypes.JSON(`{
				"url": "http://example.com/hook",
				"headers": {"Authorization": "Bearer tok-1", "X-Source": "octotify"},
				"body_template": "{\"text\":\"[{{title}}] {{content}}\",\"ts\":{{timestamp}}}"
			}`),
			title:   "告警",
			content: "CPU 90%",
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
			},
			verify: func(t *testing.T, r *http.Request, body string) {
				if r.Header.Get("Authorization") != "Bearer tok-1" {
					t.Errorf("Authorization 头缺失或不匹配: %q", r.Header.Get("Authorization"))
				}
				if r.Header.Get("X-Source") != "octotify" {
					t.Errorf("X-Source 头缺失: %q", r.Header.Get("X-Source"))
				}
				if !strings.Contains(body, `"text":"[告警] CPU 90%"`) {
					t.Errorf("模板渲染结果不符: %s", body)
				}
				if !strings.Contains(body, `"ts":`) {
					t.Errorf("timestamp 占位符未替换: %s", body)
				}
			},
		},
		{
			name: "成功_行式请求头_全角冒号",
			config: datatypes.JSON(`{
				"url": "http://example.com/hook",
				"headers": "Authorization: Bearer abc\nX-Token：xyz"
			}`),
			title:   "t",
			content: "c",
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
			},
			verify: func(t *testing.T, r *http.Request, body string) {
				if r.Header.Get("Authorization") != "Bearer abc" {
					t.Errorf("行式请求头解析失败: %q", r.Header.Get("Authorization"))
				}
				if r.Header.Get("X-Token") != "xyz" {
					t.Errorf("全角冒号解析失败: %q", r.Header.Get("X-Token"))
				}
			},
		},
		{
			name: "成功_form类型_URL转义",
			config: datatypes.JSON(`{
				"url": "http://example.com/hook",
				"content_type": "application/x-www-form-urlencoded"
			}`),
			title:   "a&b=c",
			content: "中文 值 + 空格",
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
			},
			verify: func(t *testing.T, r *http.Request, body string) {
				if ct := r.Header.Get("Content-Type"); ct != "application/x-www-form-urlencoded" {
					t.Errorf("Content-Type 不符: %s", ct)
				}
				if !strings.Contains(body, "title=a%26b%3Dc") {
					t.Errorf("title 未做 URL 转义: %s", body)
				}
				if !strings.Contains(body, "content=") {
					t.Errorf("content 缺失: %s", body)
				}
			},
		},
		{
			name: "成功_text类型_原样替换",
			config: datatypes.JSON(`{
				"url": "http://example.com/hook",
				"content_type": "text/plain",
				"body_template": "{{title}}\n====\n{{content}}"
			}`),
			title:   "标题",
			content: "内容<特殊>字符不转义",
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
			},
			verify: func(t *testing.T, r *http.Request, body string) {
				if !strings.Contains(body, "标题\n====\n内容<特殊>字符不转义") {
					t.Errorf("text 模板应原样替换: %q", body)
				}
			},
		},
		{
			name:    "成功_PUT方法",
			config:  datatypes.JSON(`{"url":"http://example.com/hook","method":"put"}`),
			title:   "t",
			content: "c",
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
			},
			verify: func(t *testing.T, r *http.Request, _ string) {
				if r.Method != http.MethodPut {
					t.Errorf("方法应为 PUT，got %s", r.Method)
				}
			},
		},
		{
			name:    "失败_缺url",
			config:  datatypes.JSON(`{}`),
			title:   "t",
			content: "c",
			handler: nil,
			wantErr: true,
		},
		{
			name:        "失败_非法方法",
			config:      datatypes.JSON(`{"url":"http://example.com","method":"DELETE"}`),
			title:       "t",
			content:     "c",
			handler:     nil,
			wantErr:     true,
			errContains: "不支持的请求方法",
		},
		{
			name:        "失败_非法content_type",
			config:      datatypes.JSON(`{"url":"http://example.com","content_type":"text/html"}`),
			title:       "t",
			content:     "c",
			handler:     nil,
			wantErr:     true,
			errContains: "不支持的 Content-Type",
		},
		{
			name:        "失败_模板渲染后非法JSON",
			config:      datatypes.JSON(`{"url":"http://example.com","body_template":"{not-json {{title}}"}`),
			title:       "t",
			content:     "c",
			handler:     nil,
			wantErr:     true,
			errContains: "合法 JSON",
		},
		{
			name:        "失败_行式请求头缺冒号",
			config:      datatypes.JSON(`{"url":"http://example.com","headers":"无效行没有冒号"}`),
			title:       "t",
			content:     "c",
			handler:     nil,
			wantErr:     true,
			errContains: "Key: Value",
		},
		{
			name:    "失败_HTTP500",
			config:  datatypes.JSON(`{"url":"http://example.com/hook"}`),
			title:   "t",
			content: "c",
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusInternalServerError)
				w.Write([]byte("server exploded"))
			},
			wantErr:     true,
			errContains: "500",
		},
		{
			name:    "失败_配置非法JSON",
			config:  datatypes.JSON(`{invalid}`),
			title:   "t",
			content: "c",
			handler: nil,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var gotReq *http.Request
			var gotBody string
			var srv *httptest.Server
			cfg := tt.config

			if tt.handler != nil {
				srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					b, _ := io.ReadAll(r.Body)
					gotReq = r
					gotBody = string(b)
					tt.handler(w, r)
				}))
				defer srv.Close()
				// 将模板里的 example.com 替换为测试服务器地址
				cfg = datatypes.JSON(strings.ReplaceAll(string(tt.config), "http://example.com", srv.URL))
			}

			s := NewWebhookSender(zaptest.NewLogger(t))
			err := s.Send(context.Background(), cfg, tt.title, tt.content)

			if tt.wantErr {
				if err == nil {
					t.Fatal("期望返回错误，实际为 nil")
				}
				if tt.errContains != "" && !strings.Contains(err.Error(), tt.errContains) {
					t.Fatalf("错误信息应包含 %q，got: %v", tt.errContains, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("不期望的错误: %v", err)
			}
			if tt.verify != nil {
				tt.verify(t, gotReq, gotBody)
			}
		})
	}
}

func TestWebhookSender_TimeoutClamp(t *testing.T) {
	// timeout_sec 超上限应被收敛到 120 且不影响正常发送
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	s := NewWebhookSender(zaptest.NewLogger(t))
	cfg := datatypes.JSON(`{"url":"` + srv.URL + `","timeout_sec":99999}`)
	if err := s.Send(context.Background(), cfg, "t", "c"); err != nil {
		t.Fatalf("超时收敛后应发送成功: %v", err)
	}
}
