package command

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"log"
	"time"

	"mgate-cloud/internal/api"
	"mgate-cloud/internal/audit"
	"mgate-cloud/internal/db"
	"mgate-cloud/internal/model"
	"mgate-cloud/internal/util"
)

// DeviceGate 抽象"设备是否允许下发命令"及在线判断，由 device.Service 实现。
//
// 用接口而非直接依赖具体类型，便于命令服务单测时注入桩。返回的 error 应为 *api.Error。
type DeviceGate interface {
	EnsureCommandable(ctx context.Context, deviceID string) (model.Device, error)
	IsOnline(deviceID string) bool
}

// Settings 是命令服务的配置快照。
type Settings struct {
	DefaultTimeout    time.Duration
	MaxTimeout        time.Duration
	ResultMaxBytes    int
	PendingTTL        time.Duration
	DefaultMaxAttempt int           // 命令默认最大投递尝试次数（Phase 5 默认 3）
	LeaseDuration     time.Duration // 投递租约时长
	RetryBackoff      time.Duration // 重试退避
	PullMaxCommands   int           // 单次 Pull 返回命令上限
}

// 命令创建后的投递提示，返回给前端用于展示。
const (
	HintDeliveredViaWS     = "delivered_via_ws"
	HintQueuedForRetry     = "queued_for_retry"
	HintOfflineWaitForPull = "device_offline_waiting_for_pull"
)

// maxDispatchPerReconnect 限制单次 WS 重连后批量投递的命令数，避免突发风暴。
const maxDispatchPerReconnect = 50

// Service 编排命令的创建、投递、ack/result 回收、取消与超时清理。
type Service struct {
	db         *sql.DB
	clock      util.Clock
	store      *Store
	results    *ResultStore
	dispatcher *Dispatcher
	devices    DeviceGate
	audit      *audit.Service
	cfg        Settings
}

// NewService 构造命令服务。
func NewService(database *sql.DB, clock util.Clock, store *Store, results *ResultStore, dispatcher *Dispatcher, devices DeviceGate, auditService *audit.Service, cfg Settings) *Service {
	return &Service{
		db: database, clock: clock,
		store: store, results: results, dispatcher: dispatcher,
		devices: devices, audit: auditService, cfg: cfg,
	}
}

// CreateInput 是创建命令的入参（来自管理员）。
type CreateInput struct {
	DeviceID   string
	AdminID    string
	Action     string
	RawParams  []byte
	TimeoutSec int
	// 审计上下文。
	IP        string
	UserAgent string
	RequestID string
}

// CreateCommand 校验并创建命令，落库后视在线情况尝试立即 WS 投递。
//
// Phase 5：enabled 设备无论在线与否都可创建命令。
//   - 在线且 WS 投递成功 → delivered_via_ws
//   - 在线但投递失败    → queued_for_retry（保持 pending，等待重试）
//   - 离线              → device_offline_waiting_for_pull（等待 Pull 或 WS 重连）
//
// 顺序保证：先校验 → 落库（pending）→ 写 create 审计 → 视情况投递。命令一定先落库再投递。
func (s *Service) CreateCommand(ctx context.Context, in CreateInput) (model.Command, string, error) {
	dev, err := s.devices.EnsureCommandable(ctx, in.DeviceID)
	if err != nil {
		return model.Command{}, "", err
	}

	if !IsAllowed(in.Action) {
		return model.Command{}, "", api.ErrInvalidAction
	}
	params, err := ValidateParams(in.Action, in.RawParams)
	if err != nil {
		return model.Command{}, "", api.ErrInvalidParams
	}

	timeout, err := s.resolveTimeout(in.TimeoutSec)
	if err != nil {
		return model.Command{}, "", err
	}

	maxAttempts := s.cfg.DefaultMaxAttempt
	if maxAttempts < 1 {
		maxAttempts = 1
	}

	now := s.clock.Now()
	expires := now.Add(s.cfg.PendingTTL)
	cmd := model.Command{
		ID:               "cmd_" + util.NewID(),
		DeviceID:         dev.ID,
		Action:           in.Action,
		ParamsJSON:       string(params),
		Status:           model.CommandStatusPending,
		CreatedByAdminID: in.AdminID,
		Priority:         100,
		TimeoutSec:       int(timeout.Seconds()),
		Attempts:         0,
		MaxAttempts:      maxAttempts,
		ExpiresAt:        &expires,
		CreatedAt:        now,
		UpdatedAt:        now,
	}

	// 先落库。
	if err := s.store.Insert(ctx, s.db, cmd); err != nil {
		return model.Command{}, "", err
	}
	ac := auditEntryCtx{ip: in.IP, ua: in.UserAgent, requestID: in.RequestID}
	s.writeAudit(ctx, model.ActorTypeAdmin, in.AdminID, model.ActionCommandCreate, cmd.ID, cmd.DeviceID, ac, map[string]any{
		"action": cmd.Action, "timeout_sec": cmd.TimeoutSec,
	})

	// 离线则保持 pending，等待 Pull / WS 重连。
	hint := HintOfflineWaitForPull
	if s.devices.IsOnline(dev.ID) {
		delivered, derr := s.dispatcher.TryDeliver(ctx, cmd)
		if derr != nil {
			log.Printf("command: WS 投递失败 command_id=%s: %v", cmd.ID, derr)
		}
		if delivered {
			hint = HintDeliveredViaWS
			s.writeAudit(ctx, model.ActorTypeAdmin, in.AdminID, model.ActionCommandDeliver, cmd.ID, cmd.DeviceID, ac, map[string]any{"action": cmd.Action})
		} else {
			hint = HintQueuedForRetry
		}
	}

	fresh, err := s.store.GetByID(ctx, s.db, cmd.ID)
	if err != nil {
		return cmd, hint, nil
	}
	return fresh, hint, nil
}

// DispatchPending 在 WS 重连后尝试投递该设备的待处理命令（与 Pull 共用 lease，不会重复）。
func (s *Service) DispatchPending(ctx context.Context, deviceID string) {
	now := s.clock.Now()
	cmds, err := s.store.ListPendingForDevice(ctx, s.db, deviceID, now, maxDispatchPerReconnect)
	if err != nil {
		log.Printf("command: 列出待投递命令失败 device_id=%s: %v", deviceID, err)
		return
	}
	for _, c := range cmds {
		delivered, derr := s.dispatcher.TryDeliver(ctx, c)
		if derr != nil {
			log.Printf("command: 重连投递失败 command_id=%s: %v", c.ID, derr)
			continue
		}
		if delivered {
			s.writeAudit(ctx, model.ActorTypeSystem, "", model.ActionCommandDeliver, c.ID, c.DeviceID, auditEntryCtx{}, map[string]any{
				"action": c.Action, "channel": "ws",
			})
		}
	}
}

// LeaseForPull 为 Pull 请求领取并标记若干 pending 命令为 sent，返回投递载荷。
//
// 与 WS 共用 lease 机制：同一命令不会被 WS 与 Pull 同时领取。
func (s *Service) LeaseForPull(ctx context.Context, deviceID, requestID string, limit int) ([]DeliverPayload, error) {
	if limit <= 0 {
		limit = 1
	}
	if s.cfg.PullMaxCommands > 0 && limit > s.cfg.PullMaxCommands {
		limit = s.cfg.PullMaxCommands
	}

	now := s.clock.Now()
	leaseUntil := now.Add(s.cfg.LeaseDuration)

	var out []DeliverPayload
	err := db.WithTx(ctx, s.db, func(tx *sql.Tx) error {
		cmds, err := s.store.LeasePendingCommands(ctx, tx, deviceID, "pull:"+requestID, leaseUntil, now, limit)
		if err != nil {
			return err
		}
		for _, c := range cmds {
			if _, err := s.store.MarkSent(ctx, tx, c.ID, now); err != nil {
				return err
			}
			out = append(out, DeliverPayload{
				CommandID:  c.ID,
				Action:     c.Action,
				Params:     json.RawMessage(c.ParamsJSON),
				TimeoutSec: c.TimeoutSec,
			})
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// HandleAck 处理 agent 的 command.ack。deviceID 为连接已认证的设备。
func (s *Service) HandleAck(ctx context.Context, deviceID string, in AckInput, ip, ua, requestID string) {
	cmd, err := s.store.GetByID(ctx, s.db, in.CommandID)
	if errors.Is(err, errCommandNotFound) {
		log.Printf("command: ack 指向未知命令 command_id=%s", in.CommandID)
		return
	}
	if err != nil {
		log.Printf("command: 查询命令失败: %v", err)
		return
	}
	// 安全：agent 只能确认属于自己的命令。
	if cmd.DeviceID != deviceID {
		log.Printf("command: ack 设备不匹配 command_id=%s", in.CommandID)
		return
	}

	auditCtx := auditEntryCtx{ip: ip, ua: ua, requestID: requestID}

	if in.Accepted {
		if _, err := s.store.MarkAcked(ctx, s.db, cmd.ID, s.clock.Now()); err != nil {
			log.Printf("command: 标记 acked 失败: %v", err)
			return
		}
		s.writeAudit(ctx, model.ActorTypeDevice, cmd.DeviceID, model.ActionCommandAck, cmd.ID, cmd.DeviceID, auditCtx, map[string]any{"action": cmd.Action})
		return
	}

	// 被拒绝：置为 failed 并记录拒绝原因（不覆盖已终态命令）。
	now := s.clock.Now()
	_ = db.WithTx(ctx, s.db, func(tx *sql.Tx) error {
		if _, err := s.store.ApplyResultStatus(ctx, tx, cmd.ID, model.CommandStatusFailed, nil, &now, now); err != nil {
			return err
		}
		msg := truncateMsg(in.Message)
		_, err := s.results.InsertIfAbsent(ctx, tx, model.CommandResult{
			CommandID:    cmd.ID,
			DeviceID:     cmd.DeviceID,
			Status:       model.CommandStatusFailed,
			ErrorMessage: &msg,
			FinishedAt:   &now,
			ReceivedAt:   now,
		})
		return err
	})
	s.writeAudit(ctx, model.ActorTypeDevice, cmd.DeviceID, model.ActionCommandRejected, cmd.ID, cmd.DeviceID, auditCtx, map[string]any{
		"action": cmd.Action, "reason": truncateMsg(in.Message),
	})
}

// HandleResult 处理 agent 的 command.result。
func (s *Service) HandleResult(ctx context.Context, deviceID string, in ResultInput, ip, ua, requestID string) {
	cmd, err := s.store.GetByID(ctx, s.db, in.CommandID)
	if errors.Is(err, errCommandNotFound) {
		log.Printf("command: result 指向未知命令 command_id=%s", in.CommandID)
		return
	}
	if err != nil {
		log.Printf("command: 查询命令失败: %v", err)
		return
	}
	if cmd.DeviceID != deviceID {
		log.Printf("command: result 设备不匹配 command_id=%s", in.CommandID)
		return
	}

	// 规整状态：非法状态一律按 failed 处理。
	status := in.Status
	if !isValidResultStatus(status) {
		status = model.CommandStatusFailed
	}

	now := s.clock.Now()
	stdout, t1 := truncate(in.Stdout, s.cfg.ResultMaxBytes)
	stderr, t2 := truncate(in.Stderr, s.cfg.ResultMaxBytes)
	resultJSON, t3 := truncate(string(in.Result), s.cfg.ResultMaxBytes)
	truncated := t1 || t2 || t3

	rec := model.CommandResult{
		CommandID:    cmd.ID,
		DeviceID:     cmd.DeviceID,
		Status:       status,
		ExitCode:     in.ExitCode,
		Stdout:       ptrOrNil(stdout),
		Stderr:       ptrOrNil(stderr),
		ResultJSON:   ptrOrNil(resultJSON),
		ErrorMessage: ptrOrNil(truncateMsg(in.ErrorMessage)),
		Truncated:    truncated,
		StartedAt:    in.StartedAt,
		FinishedAt:   in.FinishedAt,
		ReceivedAt:   now,
	}

	var inserted bool
	_ = db.WithTx(ctx, s.db, func(tx *sql.Tx) error {
		ok, err := s.results.InsertIfAbsent(ctx, tx, rec)
		if err != nil {
			return err
		}
		inserted = ok
		// 仅在命令尚未终态时更新状态；canceled/expired/timeout 等终态不被覆盖。
		if _, err := s.store.ApplyResultStatus(ctx, tx, cmd.ID, status, in.StartedAt, in.FinishedAt, now); err != nil {
			return err
		}
		return nil
	})

	// 审计仅记录摘要，绝不写 stdout/stderr 全量。
	if inserted {
		s.writeAudit(ctx, model.ActorTypeDevice, cmd.DeviceID, model.ActionCommandResult, cmd.ID, cmd.DeviceID, auditEntryCtx{ip: ip, ua: ua, requestID: requestID}, map[string]any{
			"action": cmd.Action, "status": status, "exit_code": in.ExitCode, "truncated": truncated,
		})
	}
}

// CancelCommand 由管理员取消命令（cloud 侧标记，不通知 agent）。
func (s *Service) CancelCommand(ctx context.Context, commandID, adminID, ip, ua, requestID string) error {
	cmd, err := s.store.GetByID(ctx, s.db, commandID)
	if errors.Is(err, errCommandNotFound) {
		return api.ErrCommandNotFound
	}
	if err != nil {
		return err
	}
	canceled, err := s.store.Cancel(ctx, s.db, commandID, s.clock.Now())
	if err != nil {
		return err
	}
	if !canceled {
		return api.ErrCommandNotCancelable
	}
	s.writeAudit(ctx, model.ActorTypeAdmin, adminID, model.ActionCommandCancel, cmd.ID, cmd.DeviceID, auditEntryCtx{ip: ip, ua: ua, requestID: requestID}, map[string]any{"action": cmd.Action})
	return nil
}

// ListCommands 返回命令列表。
func (s *Service) ListCommands(ctx context.Context, f ListFilter) ([]ListItem, error) {
	return s.store.List(ctx, s.db, f)
}

// GetCommandDetail 返回命令及其结果（若有）。
func (s *Service) GetCommandDetail(ctx context.Context, commandID string) (model.Command, *model.CommandResult, error) {
	cmd, err := s.store.GetByID(ctx, s.db, commandID)
	if errors.Is(err, errCommandNotFound) {
		return model.Command{}, nil, api.ErrCommandNotFound
	}
	if err != nil {
		return model.Command{}, nil, err
	}
	res, found, err := s.results.GetByCommandID(ctx, s.db, commandID)
	if err != nil {
		return model.Command{}, nil, err
	}
	if !found {
		return cmd, nil, nil
	}
	return cmd, &res, nil
}

// resolveTimeout 计算有效超时：缺省用默认值，超过上限报错。
func (s *Service) resolveTimeout(sec int) (time.Duration, error) {
	if sec <= 0 {
		return s.cfg.DefaultTimeout, nil
	}
	d := time.Duration(sec) * time.Second
	if d > s.cfg.MaxTimeout {
		return 0, api.ErrTimeoutTooLarge
	}
	return d, nil
}

// truncateMsg 限制错误/拒绝消息长度，避免异常超长文本入库。
func truncateMsg(s string) string {
	const maxMsg = 1024
	out, _ := truncate(s, maxMsg)
	return out
}

// ptrOrNil 把空串转为 nil 指针（用于可空列语义）。
func ptrOrNil(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
