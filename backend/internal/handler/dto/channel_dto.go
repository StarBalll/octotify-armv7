package dto

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"

	"gorm.io/datatypes"
)

// ChannelConfig is a map type that serializes to/from JSON for database storage.
// It generates `additionalProperties: true` in OpenAPI spec, which maps to `Record<string, unknown>` in TypeScript.
type ChannelConfig map[string]any

// Scan implements sql.Scanner interface for GORM database reading.
func (c *ChannelConfig) Scan(value any) error {
	if value == nil {
		*c = nil
		return nil
	}
	var jsonBytes []byte
	switch v := value.(type) {
	case []byte:
		jsonBytes = v
	case string:
		jsonBytes = []byte(v)
	default:
		return fmt.Errorf("cannot scan type %T into ChannelConfig", value)
	}
	return json.Unmarshal(jsonBytes, c)
}

// Value implements driver.Valuer interface for GORM database writing.
func (c ChannelConfig) Value() (driver.Value, error) {
	if c == nil {
		return nil, nil
	}
	return json.Marshal(c)
}

// ToJSON converts ChannelConfig to datatypes.JSON for internal use.
func (c ChannelConfig) ToJSON() datatypes.JSON {
	if c == nil {
		return datatypes.JSON("{}")
	}
	b, _ := json.Marshal(c)
	return datatypes.JSON(b)
}

// FromJSON converts datatypes.JSON to ChannelConfig.
func FromJSON(j datatypes.JSON) ChannelConfig {
	var cfg ChannelConfig
	if j == nil {
		return cfg
	}
	_ = json.Unmarshal(j, &cfg)
	return cfg
}

const (
	ChannelTypeWechat   = "wechat"
	ChannelTypeTelegram = "telegram"
	ChannelTypeDingtalk = "dingtalk"
	ChannelTypeEmail    = "email"
	ChannelTypeWebhook  = "webhook"
	ChannelTypeFeishu   = "feishu"
	ChannelTypeGotify   = "gotify"
)

var ValidChannelTypes = map[string]bool{
	ChannelTypeWechat:   true,
	ChannelTypeTelegram: true,
	ChannelTypeDingtalk: true,
	ChannelTypeEmail:    true,
	ChannelTypeWebhook:  true,
	ChannelTypeFeishu:   true,
	ChannelTypeGotify:   true,
}

// ConfigField 渠道配置字段定义
type ConfigField struct {
	Name        string `json:"name"`        // 字段名称
	Label       string `json:"label"`       // 字段标签
	Type        string `json:"type"`        // 字段类型：string, number, password, url
	Required    bool   `json:"required"`    // 是否必填
	Placeholder string `json:"placeholder"` // 占位符提示
}

// ChannelTypeMeta 渠道类型元数据
type ChannelTypeMeta struct {
	Type         string        `json:"type"`          // 渠道类型标识
	Name         string        `json:"name"`          // 渠道类型名称
	Description  string        `json:"description"`   // 描述
	ConfigFields []ConfigField `json:"config_fields"` // 配置字段定义
}

// ChannelTypeMetas 所有支持的渠道类型元数据
var ChannelTypeMetas = []ChannelTypeMeta{
	{
		Type:        ChannelTypeFeishu,
		Name:        "飞书",
		Description: "飞书自定义机器人",
		ConfigFields: []ConfigField{
			{
				Name:        "webhook_url",
				Label:       "Webhook 地址",
				Type:        "url",
				Required:    true,
				Placeholder: "https://open.feishu.cn/open-apis/bot/v2/hook/xxx",
			},
			{
				Name:        "secret",
				Label:       "签名密钥（可选）",
				Type:        "password",
				Required:    false,
				Placeholder: "用于签名校验的密钥",
			},
		},
	},
	{
		Type:        ChannelTypeDingtalk,
		Name:        "钉钉",
		Description: "钉钉群机器人",
		ConfigFields: []ConfigField{
			{
				Name:        "webhook_url",
				Label:       "Webhook 地址",
				Type:        "url",
				Required:    true,
				Placeholder: "https://oapi.dingtalk.com/robot/send?access_token=xxx",
			},
			{
				Name:        "secret",
				Label:       "加签密钥",
				Type:        "password",
				Required:    true,
				Placeholder: "SECxxxxxxxxxxxxxxxxx",
			},
		},
	},
	{
		Type:        ChannelTypeTelegram,
		Name:        "Telegram",
		Description: "Telegram Bot 推送",
		ConfigFields: []ConfigField{
			{
				Name:        "bot_token",
				Label:       "Bot Token",
				Type:        "password",
				Required:    true,
				Placeholder: "1234567890:ABCdefGHIjklMNOpqrsTUVwxyz",
			},
			{
				Name:        "chat_id",
				Label:       "Chat ID",
				Type:        "string",
				Required:    true,
				Placeholder: "私聊/群组填数字ID，频道填 @频道名",
			},
			{
				Name:        "proxy",
				Label:       "HTTP 代理（可选）",
				Type:        "url",
				Required:    false,
				Placeholder: "http://127.0.0.1:7890",
			},
		},
	},
	{
		Type:        ChannelTypeEmail,
		Name:        "邮件",
		Description: "邮件推送",
		ConfigFields: []ConfigField{
			{
				Name:        "smtp_host",
				Label:       "SMTP 服务器",
				Type:        "string",
				Required:    true,
				Placeholder: "smtp.example.com",
			},
			{
				Name:        "smtp_port",
				Label:       "SMTP 端口",
				Type:        "number",
				Required:    true,
				Placeholder: "587",
			},
			{
				Name:        "username",
				Label:       "用户名",
				Type:        "string",
				Required:    true,
				Placeholder: "user@example.com",
			},
			{
				Name:        "password",
				Label:       "密码/授权码",
				Type:        "password",
				Required:    true,
				Placeholder: "邮箱密码或授权码",
			},
			{
				Name:        "to",
				Label:       "收件人",
				Type:        "string",
				Required:    true,
				Placeholder: "recipient@example.com",
			},
			{
				Name:        "cc",
				Label:       "抄送人（可选）",
				Type:        "string",
				Required:    false,
				Placeholder: "cc1@example.com, cc2@example.com",
			},
			{
				Name:        "from_name",
				Label:       "发件人名称（可选）",
				Type:        "string",
				Required:    false,
				Placeholder: "系统通知",
			},
			{
				Name:        "skip_tls_verify",
				Label:       "跳过 TLS 证书校验",
				Type:        "boolean",
				Required:    false,
				Placeholder: "仅自签名证书时启用",
			},
		},
	},
	{
		Type:        ChannelTypeGotify,
		Name:        "Gotify",
		Description: "Gotify 自托管消息推送",
		ConfigFields: []ConfigField{
			{
				Name:        "server_url",
				Label:       "Gotify 服务器地址",
				Type:        "url",
				Required:    true,
				Placeholder: "https://gotify.example.com",
			},
			{
				Name:        "app_token",
				Label:       "App Token",
				Type:        "password",
				Required:    true,
				Placeholder: "GotifyAppToken",
			},
			{
				Name:        "priority",
				Label:       "优先级（0-10）",
				Type:        "number",
				Required:    false,
				Placeholder: "5",
			},
		},
	},
	{
		Type:        ChannelTypeWebhook,
		Name:        "自定义 Webhook",
		Description: "通用 HTTP 推送：URL/方法/请求头/请求体模板均可自定义，占位符 {{title}}、{{content}}、{{timestamp}} 按内容类型自动转义",
		ConfigFields: []ConfigField{
			{
				Name:        "url",
				Label:       "目标地址",
				Type:        "url",
				Required:    true,
				Placeholder: "https://example.com/hook 或 http://192.168.1.10:8080/notify",
			},
			{
				Name:        "method",
				Label:       "请求方法",
				Type:        "string",
				Required:    false,
				Placeholder: "POST（默认，可选 PUT/PATCH/GET）",
			},
			{
				Name:        "content_type",
				Label:       "内容类型",
				Type:        "string",
				Required:    false,
				Placeholder: "application/json（默认，可选 x-www-form-urlencoded / text/plain）",
			},
			{
				Name:        "body_template",
				Label:       "请求体模板（可选）",
				Type:        "textarea",
				Required:    false,
				Placeholder: `{"title":"{{title}}","content":"{{content}}","ts":{{timestamp}}}；留空按内容类型使用默认模板；{{timestamp}} 为 Unix 秒（数字，不要加引号）`,
			},
			{
				Name:        "headers",
				Label:       "自定义请求头（可选）",
				Type:        "textarea",
				Required:    false,
				Placeholder: `JSON 对象，如 {"Authorization":"Bearer xxx"}；或每行一个：Authorization: Bearer xxx`,
			},
			{
				Name:        "timeout_sec",
				Label:       "超时秒数（可选）",
				Type:        "number",
				Required:    false,
				Placeholder: "30（1-120）",
			},
		},
	},
}

// CreateChannelReq 创建推送渠道请求
type CreateChannelReq struct {
	Type   string        `json:"type" binding:"required,channel_type" example:"dingtalk"` // 渠道类型，可选值：wechat, telegram, dingtalk, email, webhook
	Name   string        `json:"name" binding:"required,min=1,max=128" example:"钉钉-运维群"`  // 渠道名称，1-128 个字符
	Config ChannelConfig `json:"config" binding:"required"`                               // 渠道配置，JSON 对象，不同渠道类型配置不同
}

// UpdateChannelReq 编辑推送渠道请求
type UpdateChannelReq struct {
	Name   string        `json:"name" binding:"required,min=1,max=128" example:"钉钉-运维群"` // 渠道名称，1-128 个字符
	Config ChannelConfig `json:"config" binding:"required"`                              // 渠道配置，JSON 对象，不同渠道类型配置不同
}

// ChannelDTO 推送渠道响应
type ChannelDTO struct {
	ID         int64         `json:"id" example:"1"`                          // 渠道 ID
	UserID     int64         `json:"user_id" example:"1"`                     // 所属用户 ID
	Type       string        `json:"type" example:"dingtalk"`                 // 渠道类型：wechat, telegram, dingtalk, email, webhook
	Name       string        `json:"name" example:"钉钉-运维群"`                   // 渠道名称
	Config     ChannelConfig `json:"config"`                                  // 渠道配置，JSON 格式
	Status     int           `json:"status" example:"1"`                      // 状态：1-启用 2-停用 -1-已删除
	CreatedAt  int64         `json:"created_at_ts" example:"1714636800000"`   // 创建时间（Unix 毫秒时间戳）
	UpdatedAt  int64         `json:"updated_at_ts" example:"1714636800000"`   // 更新时间（Unix 毫秒时间戳）
	LastUsedAt int64         `json:"last_used_at_ts" example:"1714636800000"` // 最后使用时间（Unix 毫秒时间戳，0 表示未使用）
}
