package service

import (
	"context"
	"time"

	"go.uber.org/zap"
	"golang.org/x/sync/errgroup"
	"gorm.io/gorm"

	"octotify/internal/handler/dto"
	"octotify/internal/model"
	"octotify/internal/query"
	"octotify/internal/sender"
	"octotify/pkg/ctxutil"
	"octotify/pkg/xerr"
)

// MessageService 消息管理服务
type MessageService struct {
	db            *gorm.DB              // 数据库连接
	logger        *zap.Logger           // 日志记录器
	senderFactory *sender.SenderFactory // 消息发送器工厂
}

// NewMessageService 创建消息管理服务实例
func NewMessageService(db *gorm.DB, logger *zap.Logger, senderFactory *sender.SenderFactory) *MessageService {
	return &MessageService{
		db:            db,
		logger:        logger,
		senderFactory: senderFactory,
	}
}

func (s *MessageService) log(ctx context.Context) *zap.Logger {
	if rid := ctxutil.GetRequestID(ctx); rid != "" {
		return s.logger.With(zap.String("request_id", rid))
	}
	return s.logger
}

// ListMessages 查看消息列表
// 按照 UML 04-01-list-messages.puml 实现
// 查询当前用户的所有消息，按创建时间倒序排列，排除已删除的消息
func (s *MessageService) ListMessages(ctx context.Context, userID int64, pageReq *dto.PageReq) ([]*dto.MessageDTO, int64, error) {
	q := query.Use(s.db)

	// 规范化分页参数
	pageReq.Normalize()
	offset := (pageReq.Page - 1) * pageReq.PageSize

	// 查询消息总数，通过 JOIN sources 表确保只能查询到用户自己的消息
	total, err := q.Message.WithContext(ctx).
		Join(q.Source, q.Source.ID.EqCol(q.Message.SourceID)).
		Where(
			q.Source.UserID.Eq(userID),                       // 用户隔离：只能查看自己的消息
			q.Message.Status.Neq(model.MessageStatusDeleted), // 排除已删除的消息
		).
		Count()
	if err != nil {
		s.log(ctx).Error("查询消息总数失败",
			zap.Error(err),
			zap.Int64("user_id", userID),
		)
		return nil, 0, xerr.ErrMessageRecordFailed.WithInternal(err)
	}

	// 查询消息列表，按创建时间倒序排列
	messages, err := q.Message.WithContext(ctx).
		Join(q.Source, q.Source.ID.EqCol(q.Message.SourceID)).
		Where(
			q.Source.UserID.Eq(userID),
			q.Message.Status.Neq(model.MessageStatusDeleted),
		).
		Order(q.Message.CreatedAt.Desc()).
		Offset(offset).
		Limit(pageReq.PageSize).
		Find()
	if err != nil {
		s.log(ctx).Error("查询消息列表失败",
			zap.Error(err),
			zap.Int64("user_id", userID),
		)
		return nil, 0, xerr.ErrMessageRecordFailed.WithInternal(err)
	}

	// 转换为 DTO 对象
	list := make([]*dto.MessageDTO, 0, len(messages))
	for _, msg := range messages {
		list = append(list, &dto.MessageDTO{
			ID:        msg.ID,
			SourceID:  msg.SourceID,
			ChannelID: msg.ChannelID,
			Title:     msg.Title,
			Content:   msg.Content,
			Status:    msg.Status,
			CreatedAt: msg.CreatedAt.UnixMilli(),
			UpdatedAt: msg.UpdatedAt.UnixMilli(),
		})
	}

	// 批量填充来源名称/渠道名称/渠道类型（UI 列表需要展示）
	s.fillSourceAndChannelNames(ctx, list)

	s.log(ctx).Info("查询消息列表成功",
		zap.Int64("user_id", userID),
		zap.Int("page", pageReq.Page),
		zap.Int("page_size", pageReq.PageSize),
		zap.Int64("total", total),
	)

	return list, total, nil
}

// fillSourceAndChannelNames 批量填充消息列表 DTO 的来源名称/渠道名称/渠道类型。
// 列表接口的 UI 表格需要展示名称（与详情接口 GetMessageByID 行为一致）；
// 使用 IN 批量查询避免 N+1。填充失败不影响列表本身，仅记录告警日志。
func (s *MessageService) fillSourceAndChannelNames(ctx context.Context, list []*dto.MessageDTO) {
	if len(list) == 0 {
		return
	}
	q := query.Use(s.db)

	// 去重收集需要查询的来源/渠道 ID
	sourceIDs := make([]int64, 0, len(list))
	channelIDs := make([]int64, 0, len(list))
	seenSource := make(map[int64]struct{}, len(list))
	seenChannel := make(map[int64]struct{}, len(list))
	for _, m := range list {
		if _, ok := seenSource[m.SourceID]; !ok {
			seenSource[m.SourceID] = struct{}{}
			sourceIDs = append(sourceIDs, m.SourceID)
		}
		if _, ok := seenChannel[m.ChannelID]; !ok {
			seenChannel[m.ChannelID] = struct{}{}
			channelIDs = append(channelIDs, m.ChannelID)
		}
	}

	// 批量查询来源名称
	sources, err := q.Source.WithContext(ctx).
		Where(q.Source.ID.In(sourceIDs...)).
		Select(q.Source.ID, q.Source.Name).
		Find()
	if err != nil {
		s.log(ctx).Warn("批量查询来源名称失败", zap.Error(err))
	} else {
		nameByID := make(map[int64]string, len(sources))
		for _, src := range sources {
			nameByID[src.ID] = src.Name
		}
		for _, m := range list {
			m.SourceName = nameByID[m.SourceID]
		}
	}

	// 批量查询渠道名称与类型
	channels, err := q.Channel.WithContext(ctx).
		Where(q.Channel.ID.In(channelIDs...)).
		Select(q.Channel.ID, q.Channel.Name, q.Channel.Type).
		Find()
	if err != nil {
		s.log(ctx).Warn("批量查询渠道信息失败", zap.Error(err))
	} else {
		nameByID := make(map[int64]string, len(channels))
		typeByID := make(map[int64]string, len(channels))
		for _, ch := range channels {
			nameByID[ch.ID] = ch.Name
			typeByID[ch.ID] = ch.Type
		}
		for _, m := range list {
			m.ChannelName = nameByID[m.ChannelID]
			m.ChannelType = typeByID[m.ChannelID]
		}
	}
}

// FilterMessages 筛选消息记录
// 按照 UML 04-02-filter-messages.puml 实现
// 支持按来源、渠道、状态、时间范围、关键词等条件筛选
func (s *MessageService) FilterMessages(ctx context.Context, userID int64, filter *dto.MessageFilterReq) ([]*dto.MessageDTO, int64, error) {
	q := query.Use(s.db)

	// 规范化分页参数
	filter.Normalize()
	offset := (filter.Page - 1) * filter.PageSize

	// 构建基础查询，确保用户只能查看自己的消息
	baseQuery := q.Message.WithContext(ctx).
		Join(q.Source, q.Source.ID.EqCol(q.Message.SourceID)).
		Where(
			q.Source.UserID.Eq(userID),                       // 用户隔离
			q.Message.Status.Neq(model.MessageStatusDeleted), // 排除已删除
		)

	// 按来源 ID 筛选
	if filter.SourceID != nil {
		baseQuery = baseQuery.Where(q.Message.SourceID.Eq(*filter.SourceID))
	}
	// 按渠道 ID 筛选
	if filter.ChannelID != nil {
		baseQuery = baseQuery.Where(q.Message.ChannelID.Eq(*filter.ChannelID))
	}
	// 按推送状态筛选
	if filter.Status != nil {
		baseQuery = baseQuery.Where(q.Message.Status.Eq(*filter.Status))
	}
	// 按开始时间筛选
	if filter.StartDate != nil {
		baseQuery = baseQuery.Where(q.Message.CreatedAt.Gte(time.UnixMilli(*filter.StartDate)))
	}
	// 按结束时间筛选
	if filter.EndDate != nil {
		baseQuery = baseQuery.Where(q.Message.CreatedAt.Lte(time.UnixMilli(*filter.EndDate)))
	}
	// 关键词搜索：搜索标题和内容
	// 注意：使用嵌套 Where 确保 OR 条件被括号包裹，不会绕过用户隔离条件
	if filter.Keyword != "" {
		keyword := "%" + filter.Keyword + "%"
		baseQuery = baseQuery.Where(
			baseQuery.Where(q.Message.Title.Like(keyword)).
				Or(q.Message.Content.Like(keyword)),
		)
	}

	// 查询符合条件的消息总数
	total, err := baseQuery.Count()
	if err != nil {
		s.log(ctx).Error("查询消息总数失败",
			zap.Error(err),
			zap.Int64("user_id", userID),
		)
		return nil, 0, xerr.ErrMessageRecordFailed.WithInternal(err)
	}

	// 查询消息列表，按创建时间倒序排列
	messages, err := baseQuery.
		Order(q.Message.CreatedAt.Desc()).
		Offset(offset).
		Limit(filter.PageSize).
		Find()
	if err != nil {
		s.log(ctx).Error("查询消息列表失败",
			zap.Error(err),
			zap.Int64("user_id", userID),
		)
		return nil, 0, xerr.ErrMessageRecordFailed.WithInternal(err)
	}

	// 转换为 DTO 对象
	list := make([]*dto.MessageDTO, 0, len(messages))
	for _, msg := range messages {
		list = append(list, &dto.MessageDTO{
			ID:        msg.ID,
			SourceID:  msg.SourceID,
			ChannelID: msg.ChannelID,
			Title:     msg.Title,
			Content:   msg.Content,
			Status:    msg.Status,
			CreatedAt: msg.CreatedAt.UnixMilli(),
			UpdatedAt: msg.UpdatedAt.UnixMilli(),
		})
	}

	// 批量填充来源名称/渠道名称/渠道类型（UI 列表需要展示）
	s.fillSourceAndChannelNames(ctx, list)

	s.log(ctx).Info("筛选消息列表成功",
		zap.Int64("user_id", userID),
		zap.Int("page", filter.Page),
		zap.Int("page_size", filter.PageSize),
		zap.Int64("total", total),
	)

	return list, total, nil
}

// GetMessageByID 查看消息详情
// 按照 UML 04-03-view-message-detail.puml 实现
// 查询单条消息的详细信息，包含来源名称和渠道信息
func (s *MessageService) GetMessageByID(ctx context.Context, userID int64, messageID int64) (*dto.MessageDetailDTO, error) {
	q := query.Use(s.db)

	// 查询消息基本信息，通过 JOIN sources 表确保只能查询用户自己的消息
	message, err := q.Message.WithContext(ctx).
		Join(q.Source, q.Source.ID.EqCol(q.Message.SourceID)).
		Where(
			q.Source.UserID.Eq(userID),                       // 用户隔离
			q.Message.ID.Eq(messageID),                       // 指定消息 ID
			q.Message.Status.Neq(model.MessageStatusDeleted), // 排除已删除
		).
		First()
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			s.log(ctx).Warn("消息不存在",
				zap.Int64("message_id", messageID),
				zap.Int64("user_id", userID),
			)
			return nil, xerr.ErrNotFound
		}
		s.log(ctx).Error("查询消息详情失败",
			zap.Error(err),
			zap.Int64("message_id", messageID),
		)
		return nil, xerr.ErrMessageRecordFailed.WithInternal(err)
	}

	// 查询来源名称
	source, err := q.Source.WithContext(ctx).
		Where(q.Source.ID.Eq(message.SourceID)).
		Select(q.Source.Name).
		First()
	if err != nil && err != gorm.ErrRecordNotFound {
		s.log(ctx).Error("查询来源信息失败",
			zap.Error(err),
			zap.Int64("source_id", message.SourceID),
		)
		return nil, xerr.ErrMessageRecordFailed.WithInternal(err)
	}

	// 查询渠道名称和类型
	channel, err := q.Channel.WithContext(ctx).
		Where(q.Channel.ID.Eq(message.ChannelID)).
		Select(q.Channel.Name, q.Channel.Type).
		First()
	if err != nil && err != gorm.ErrRecordNotFound {
		s.log(ctx).Error("查询渠道信息失败",
			zap.Error(err),
			zap.Int64("channel_id", message.ChannelID),
		)
		return nil, xerr.ErrMessageRecordFailed.WithInternal(err)
	}

	// 组装来源和渠道信息
	var sourceName, channelName, channelType string
	if source != nil {
		sourceName = source.Name
	}
	if channel != nil {
		channelName = channel.Name
		channelType = channel.Type
	}

	// 构建消息详情 DTO
	return &dto.MessageDetailDTO{
		ID:          message.ID,
		SourceID:    message.SourceID,
		SourceName:  sourceName,
		ChannelID:   message.ChannelID,
		ChannelName: channelName,
		ChannelType: channelType,
		Title:       message.Title,
		Content:     message.Content,
		Status:      message.Status,
		CreatedAt:   message.CreatedAt.UnixMilli(),
		UpdatedAt:   message.UpdatedAt.UnixMilli(),
	}, nil
}

// DeleteMessage 删除消息记录
// 按照 UML 04-04-delete-message.puml 实现
// 采用软删除方式，将消息状态标记为已删除
func (s *MessageService) DeleteMessage(ctx context.Context, userID int64, messageID int64) error {
	q := query.Use(s.db)

	// 查询消息是否存在，并验证用户权限
	message, err := q.Message.WithContext(ctx).
		Join(q.Source, q.Source.ID.EqCol(q.Message.SourceID)).
		Where(
			q.Source.UserID.Eq(userID), // 用户隔离：只能删除自己的消息
			q.Message.ID.Eq(messageID),
		).
		First()
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			s.log(ctx).Warn("消息不存在",
				zap.Int64("message_id", messageID),
				zap.Int64("user_id", userID),
			)
			return xerr.ErrNotFound
		}
		s.log(ctx).Error("查询消息失败",
			zap.Error(err),
			zap.Int64("message_id", messageID),
		)
		return xerr.ErrMessageRecordFailed.WithInternal(err)
	}

	// 检查消息是否已被删除
	if message.Status == model.MessageStatusDeleted {
		s.log(ctx).Warn("消息已删除",
			zap.Int64("message_id", messageID),
		)
		return xerr.ErrMessageAlreadyDeleted
	}

	// 使用事务执行软删除操作
	err = q.Transaction(func(tx *query.Query) error {
		_, err := tx.Message.WithContext(ctx).
			Where(tx.Message.ID.Eq(messageID)).
			Updates(map[string]interface{}{
				"status":     model.MessageStatusDeleted, // 标记为已删除
				"updated_at": time.Now(),                 // 更新修改时间
			})
		return err
	})
	if err != nil {
		s.log(ctx).Error("删除消息失败",
			zap.Error(err),
			zap.Int64("message_id", messageID),
		)
		return xerr.ErrMessageRecordFailed.WithInternal(err)
	}

	s.log(ctx).Info("消息已删除",
		zap.Int64("message_id", messageID),
	)

	return nil
}

// PushMessage 推送消息（通过来源 Token）
// 按照 UML 05-01-push-message.d2 实现
// 通过来源 Token 验证来源有效性，查询绑定的渠道，按状态分类后并发推送到可用渠道
func (s *MessageService) PushMessage(ctx context.Context, sourceToken string, req *dto.PushMessageReq) (*dto.PushResponse, error) {
	q := query.Use(s.db)

	// 验证来源 Token 是否有效（status = 1 表示正常）
	source, err := q.Source.WithContext(ctx).
		Where(
			q.Source.Token.Eq(sourceToken),
			q.Source.Status.Eq(model.SourceStatusActive),
		).
		First()
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			s.log(ctx).Warn("来源不存在或已禁用",
				zap.String("token", sourceToken),
			)
			return nil, xerr.ErrSourceNotFound
		}
		s.log(ctx).Error("查询来源失败",
			zap.Error(err),
			zap.String("token", sourceToken),
		)
		return nil, xerr.ErrSourceQueryFailed.WithInternal(err)
	}

	// 查询来源绑定的所有渠道（不限状态）
	allChannels, err := q.Channel.WithContext(ctx).
		Join(q.SourceChannel, q.SourceChannel.ChannelID.EqCol(q.Channel.ID)).
		Where(q.SourceChannel.SourceID.Eq(source.ID)).
		Find()
	if err != nil {
		s.log(ctx).Error("查询渠道失败",
			zap.Error(err),
			zap.Int64("source_id", source.ID),
		)
		return nil, xerr.ErrChannelQueryFailed.WithInternal(err)
	}

	// 检查是否绑定了任何渠道
	if len(allChannels) == 0 {
		s.log(ctx).Warn("来源未绑定任何渠道",
			zap.Int64("source_id", source.ID),
		)
		return nil, xerr.ErrMessageNoChannels
	}

	// 查询可用渠道（status = 1 表示正常）
	activeChannels := make([]*model.Channel, 0, len(allChannels))
	hasDisabled := false
	hasDeleted := false
	for _, ch := range allChannels {
		switch ch.Status {
		case model.ChannelStatusActive:
			activeChannels = append(activeChannels, ch)
		case model.ChannelStatusDisabled:
			hasDisabled = true
		case model.ChannelStatusDeleted:
			hasDeleted = true
		}
	}

	// 检查是否有可用渠道
	if len(activeChannels) == 0 {
		if hasDeleted {
			s.log(ctx).Warn("来源绑定的渠道均已删除",
				zap.Int64("source_id", source.ID),
			)
			return nil, xerr.ErrMessageChannelsDeleted
		}
		if hasDisabled {
			s.log(ctx).Warn("来源绑定的渠道均已停用",
				zap.Int64("source_id", source.ID),
			)
			return nil, xerr.ErrMessageChannelsDisabled
		}
	}

	// 并发推送到各渠道
	return s.pushToChannels(ctx, source, activeChannels, req)
}

// pushToChannels 并发推送到多个渠道
// 按照 UML 05-02-push-multi-channels.puml 实现
// 使用 errgroup 并发控制，每个渠道独立 goroutine，单个渠道超时 30 秒
func (s *MessageService) pushToChannels(ctx context.Context, source *model.Source, channels []*model.Channel, req *dto.PushMessageReq) (*dto.PushResponse, error) {
	q := query.Use(s.db)

	// 使用 errgroup 管理并发 goroutine
	g, ctx := errgroup.WithContext(ctx)
	// 预分配结果数组，避免并发写入冲突
	results := make([]*dto.PushResult, len(channels))

	// 遍历所有渠道，为每个渠道启动独立的 goroutine 推送
	for i, ch := range channels {
		// 闭包捕获循环变量
		i, ch := i, ch
		g.Go(func() error {
			// 创建消息记录，初始状态为待推送（100）
			message := &model.Message{
				SourceID:  source.ID,
				ChannelID: ch.ID,
				Title:     req.Title,
				Content:   req.Message,
				Status:    model.MessageStatusPending,
			}

			// 使用事务创建消息记录，确保数据一致性
			err := q.Transaction(func(tx *query.Query) error {
				if err := tx.Message.WithContext(ctx).Create(message); err != nil {
					return err
				}
				return nil
			})
			if err != nil {
				// 消息记录创建失败，记录失败结果
				s.log(ctx).Error("创建消息记录失败",
					zap.Error(err),
					zap.Int64("channel_id", ch.ID),
				)
				results[i] = &dto.PushResult{
					ChannelID:   ch.ID,
					ChannelName: ch.Name,
					Success:     false,
					Error:       "创建消息记录失败",
				}
				// 返回 nil 不中断其他渠道的推送
				return nil
			}

			// 根据渠道类型创建对应的发送器
			snd, err := s.senderFactory.Create(ch.Type)
			if err != nil {
				// 不支持的渠道类型，更新消息状态为失败
				s.log(ctx).Error("创建发送器失败",
					zap.Error(err),
					zap.String("channel_type", ch.Type),
				)
				s.updateMessageStatus(ctx, message.ID, model.MessageStatusFailed)
				results[i] = &dto.PushResult{
					ChannelID:   ch.ID,
					ChannelName: ch.Name,
					MessageID:   message.ID,
					Success:     false,
					Error:       "不支持的渠道类型",
				}
				return nil
			}

			// 设置 30 秒超时，防止单个渠道阻塞整体推送
			sendCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
			defer cancel()

			// 调用渠道 API 推送消息
			err = snd.Send(sendCtx, ch.Config, req.Title, req.Message)
			if err != nil {
				// 推送失败，更新消息状态为失败（300）
				s.log(ctx).Error("推送消息失败",
					zap.Error(err),
					zap.Int64("channel_id", ch.ID),
					zap.Int64("message_id", message.ID),
				)
				s.updateMessageStatus(ctx, message.ID, model.MessageStatusFailed)
				results[i] = &dto.PushResult{
					ChannelID:   ch.ID,
					ChannelName: ch.Name,
					MessageID:   message.ID,
					Success:     false,
					Error:       err.Error(),
				}
				return nil
			}

			// 推送成功，更新消息状态为成功（200）
			s.updateMessageStatus(ctx, message.ID, model.MessageStatusSuccess)
			results[i] = &dto.PushResult{
				ChannelID:   ch.ID,
				ChannelName: ch.Name,
				MessageID:   message.ID,
				Success:     true,
			}

			s.log(ctx).Info("推送消息成功",
				zap.Int64("channel_id", ch.ID),
				zap.Int64("message_id", message.ID),
			)

			return nil
		})
	}

	// 等待所有 goroutine 完成
	if err := g.Wait(); err != nil {
		s.log(ctx).Error("并发推送消息失败",
			zap.Error(err),
		)
		return nil, xerr.ErrMessagePushFailed.WithInternal(err)
	}

	// 统计推送结果
	var successCount, failedCount int
	for _, r := range results {
		if r.Success {
			successCount++
		} else {
			failedCount++
		}
	}

	// 记录推送完成日志
	s.log(ctx).Info("消息推送完成",
		zap.Int64("source_id", source.ID),
		zap.Int("total", len(results)),
		zap.Int("success", successCount),
		zap.Int("failed", failedCount),
	)

	// 返回推送结果汇总
	return &dto.PushResponse{
		Total:   len(results),
		Success: successCount,
		Failed:  failedCount,
		Results: results,
	}, nil
}

// updateMessageStatus 更新消息推送状态
// 按照 UML 05-03-record-status.puml 实现
// 支持状态流转：100（待推送）→ 200（成功）/ 300（失败）
func (s *MessageService) updateMessageStatus(ctx context.Context, messageID int64, status int) {
	q := query.Use(s.db)

	// 使用事务更新状态，确保数据一致性
	_, err := q.Message.WithContext(ctx).
		Where(q.Message.ID.Eq(messageID)).
		Updates(map[string]interface{}{
			"status":     status,     // 更新推送状态
			"updated_at": time.Now(), // 更新修改时间
		})
	if err != nil {
		// 状态更新失败，记录错误日志（不返回错误，避免影响主流程）
		s.log(ctx).Error("更新消息状态失败",
			zap.Error(err),
			zap.Int64("message_id", messageID),
			zap.Int("status", status),
		)
	}
}
