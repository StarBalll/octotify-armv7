package dto

// PushMessageReq 推送消息请求
type PushMessageReq struct {
	Title   string `json:"title" binding:"required,min=1" example:"CI Build"`            // 消息标题，不能为空
	Message string `json:"message" binding:"required,min=1" example:"Build #123 passed"` // 消息内容，不能为空
}

// MessageDTO 消息记录响应
type MessageDTO struct {
	ID          int64  `json:"id" example:"1"`                        // 消息 ID
	SourceID    int64  `json:"source_id" example:"1"`                 // 来源 ID
	SourceName  string `json:"source_name" example:"CI Pipeline"`     // 来源名称（列表接口批量填充）
	ChannelID   int64  `json:"channel_id" example:"1"`                // 渠道 ID
	ChannelName string `json:"channel_name" example:"钉钉-运维群"`         // 渠道名称（列表接口批量填充）
	ChannelType string `json:"channel_type" example:"dingtalk"`       // 渠道类型（列表接口批量填充）
	Title       string `json:"title" example:"CI Build"`              // 消息标题
	Content     string `json:"content" example:"Build #123 passed"`   // 消息内容
	Status      int    `json:"status" example:"200"`                  // 推送状态：100-待推送 200-成功 300-失败 -1-已删除
	CreatedAt   int64  `json:"created_at_ts" example:"1714636800000"` // 创建时间（Unix 毫秒时间戳）
	UpdatedAt   int64  `json:"updated_at_ts" example:"1714636800000"` // 更新时间（Unix 毫秒时间戳）
}

// MessageDetailDTO 消息详情响应（包含来源和渠道信息）
type MessageDetailDTO struct {
	ID          int64  `json:"id" example:"1"`                        // 消息 ID
	SourceID    int64  `json:"source_id" example:"1"`                 // 来源 ID
	SourceName  string `json:"source_name" example:"CI Pipeline"`     // 来源名称
	ChannelID   int64  `json:"channel_id" example:"1"`                // 渠道 ID
	ChannelName string `json:"channel_name" example:"钉钉-运维群"`         // 渠道名称
	ChannelType string `json:"channel_type" example:"dingtalk"`       // 渠道类型：wechat, telegram, dingtalk, email, webhook
	Title       string `json:"title" example:"CI Build"`              // 消息标题
	Content     string `json:"content" example:"Build #123 passed"`   // 消息内容
	Status      int    `json:"status" example:"200"`                  // 推送状态：100-待推送 200-成功 300-失败 -1-已删除
	CreatedAt   int64  `json:"created_at_ts" example:"1714636800000"` // 创建时间（Unix 毫秒时间戳）
	UpdatedAt   int64  `json:"updated_at_ts" example:"1714636800000"` // 更新时间（Unix 毫秒时间戳）
}

// MessageFilterReq 消息筛选请求参数
type MessageFilterReq struct {
	PageReq
	SourceID  *int64 `form:"source_id" example:"1"`              // 来源 ID（可选）
	ChannelID *int64 `form:"channel_id" example:"1"`             // 渠道 ID（可选）
	Status    *int   `form:"status" example:"200"`               // 推送状态（可选）：100-待推送 200-成功 300-失败
	StartDate *int64 `form:"start_date" example:"1714636800000"` // 开始时间（可选，Unix 毫秒）
	EndDate   *int64 `form:"end_date" example:"1714723200000"`   // 结束时间（可选，Unix 毫秒）
	Keyword   string `form:"keyword" example:"Build"`            // 关键词（可选，搜索标题和内容）
}

// PushResult 单渠道推送结果
type PushResult struct {
	ChannelID   int64  `json:"channel_id"`      // 渠道 ID
	ChannelName string `json:"channel_name"`    // 渠道名称
	MessageID   int64  `json:"message_id"`      // 消息 ID
	Success     bool   `json:"success"`         // 是否推送成功
	Error       string `json:"error,omitempty"` // 失败时的错误信息
}

// PushResponse 推送响应
type PushResponse struct {
	Total   int           `json:"total"`   // 推送渠道总数
	Success int           `json:"success"` // 推送成功数
	Failed  int           `json:"failed"`  // 推送失败数
	Results []*PushResult `json:"results"` // 各渠道推送结果详情
}
