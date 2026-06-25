package command

import (
	"context"
	"log"
	"time"

	"mgate-cloud/internal/audit"
	"mgate-cloud/internal/model"
)

// auditEntryCtx 承载审计的请求上下文字段。
type auditEntryCtx struct {
	ip        string
	ua        string
	requestID string
}

// writeAudit 写一条命令相关审计，统一填充 target 与上下文。
func (s *Service) writeAudit(ctx context.Context, actorType, actorID, action, commandID, deviceID string, ac auditEntryCtx, metadata map[string]any) {
	if metadata == nil {
		metadata = map[string]any{}
	}
	metadata["device_id"] = deviceID
	s.audit.Record(ctx, audit.Entry{
		ActorType:  actorType,
		ActorID:    actorID,
		Action:     action,
		TargetType: model.TargetTypeCommand,
		TargetID:   commandID,
		IP:         ac.ip,
		UserAgent:  ac.ua,
		RequestID:  ac.requestID,
		Summary:    action,
		Metadata:   metadata,
	})
}

// RunReaper 周期扫描超时/过期命令，直到 ctx 取消（用于服务优雅关闭）。
func (s *Service) RunReaper(ctx context.Context, interval time.Duration) {
	if interval <= 0 {
		interval = 10 * time.Second
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.ReapOnce(ctx)
		}
	}
}

// ReapOnce 执行一轮 lease 回收、超时重试与过期清理。可单独调用以便测试。
//
//   - 卡住的命令（leased 租约过期，或 sent/acked/running 超过 timeout_sec）：
//     若 attempts < max_attempts → 退回 pending 重试（command.retry）；否则 → timeout。
//   - pending 命令超过 expires_at → expired。
//   - 终态命令不处理。
func (s *Service) ReapOnce(ctx context.Context) {
	now := s.clock.Now()

	active, err := s.store.FindActive(ctx, s.db)
	if err != nil {
		log.Printf("command: 扫描进行中命令失败: %v", err)
	}
	for _, cmd := range active {
		if !isStuck(cmd, now) {
			continue
		}
		if cmd.Attempts < cmd.MaxAttempts {
			// 仍可重试：退回 pending，设置退避，等待下次 WS/Pull 领取。
			retryAfter := now.Add(s.cfg.RetryBackoff)
			ok, err := s.store.RetryToPending(ctx, s.db, cmd.ID, "delivery lease/timeout, retry", retryAfter, now)
			if err != nil {
				log.Printf("command: 重试退回失败 command_id=%s: %v", cmd.ID, err)
				continue
			}
			if ok {
				s.writeAudit(ctx, model.ActorTypeSystem, "", model.ActionCommandRetry, cmd.ID, cmd.DeviceID, auditEntryCtx{}, map[string]any{
					"action": cmd.Action, "attempts": cmd.Attempts, "max_attempts": cmd.MaxAttempts,
				})
			}
			continue
		}
		// 重试次数耗尽：标记超时。
		ok, err := s.store.MarkTimeout(ctx, s.db, cmd.ID, now)
		if err != nil {
			log.Printf("command: 标记超时失败 command_id=%s: %v", cmd.ID, err)
			continue
		}
		if ok {
			s.writeAudit(ctx, model.ActorTypeSystem, "", model.ActionCommandTimeout, cmd.ID, cmd.DeviceID, auditEntryCtx{}, map[string]any{
				"action": cmd.Action, "attempts": cmd.Attempts,
			})
		}
	}

	expired, err := s.store.FindExpiredPending(ctx, s.db, now)
	if err != nil {
		log.Printf("command: 扫描过期命令失败: %v", err)
	}
	for _, cmd := range expired {
		ok, err := s.store.MarkExpired(ctx, s.db, cmd.ID, now)
		if err != nil {
			log.Printf("command: 标记过期失败 command_id=%s: %v", cmd.ID, err)
			continue
		}
		if ok {
			s.writeAudit(ctx, model.ActorTypeSystem, "", model.ActionCommandExpired, cmd.ID, cmd.DeviceID, auditEntryCtx{}, map[string]any{
				"action": cmd.Action,
			})
		}
	}
}

// isStuck 判断进行中的命令是否需要 reaper 介入（重试或超时）。
//
//   - leased：投递租约已过期（领取后未能完成投递/或投递后长期无进展）。
//   - sent/acked/running：自处理起点起超过 timeout_sec 仍无终态。
func isStuck(cmd model.Command, now time.Time) bool {
	switch cmd.Status {
	case model.CommandStatusLeased:
		return cmd.LeaseUntil != nil && !now.Before(*cmd.LeaseUntil)
	case model.CommandStatusSent, model.CommandStatusAcked, model.CommandStatusRunning:
		start := effectiveStart(cmd)
		if start.IsZero() {
			return cmd.LeaseUntil != nil && !now.Before(*cmd.LeaseUntil)
		}
		return now.Sub(start) > time.Duration(cmd.TimeoutSec)*time.Second
	default:
		return false
	}
}

func effectiveStart(cmd model.Command) time.Time {
	switch {
	case cmd.StartedAt != nil:
		return *cmd.StartedAt
	case cmd.AckedAt != nil:
		return *cmd.AckedAt
	case cmd.SentAt != nil:
		return *cmd.SentAt
	default:
		return time.Time{}
	}
}
