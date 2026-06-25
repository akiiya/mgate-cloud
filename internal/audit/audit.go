// Package audit 提供审计日志的写入框架。
//
// Phase 1 仅落地登录、登出、bootstrap 等安全相关事件，但接口设计为通用的
// Record，后续新增事件无需改动框架本身。
package audit

import (
	"context"
	"database/sql"
	"encoding/json"
	"log"

	"mgate-cloud/internal/util"
)

// Entry 描述一条待记录的审计事件。
//
// 用值对象承载入参，相比一长串位置参数更可读、更易扩展。
type Entry struct {
	ActorType  string
	ActorID    string
	Action     string
	TargetType string
	TargetID   string
	IP         string
	UserAgent  string
	RequestID  string
	Summary    string
	// Metadata 为结构化补充信息，写库前会经 util.Redact 脱敏。
	Metadata map[string]any
}

// Service 负责审计日志的持久化。
type Service struct {
	db    *sql.DB
	clock util.Clock
}

// NewService 构造审计服务。
func NewService(db *sql.DB, clock util.Clock) *Service {
	return &Service{db: db, clock: clock}
}

// Record 写入一条审计日志。
//
// 重要约定：审计写入失败【绝不】向上抛出或中断业务主流程（如登录成功流程），
// 仅记录普通日志告警。审计是旁路观测，不能因它的故障拖垮核心功能。
func (s *Service) Record(ctx context.Context, e Entry) {
	metadataJSON := s.encodeMetadata(e.Metadata)

	const q = `
		INSERT INTO audit_logs
			(id, actor_type, actor_id, action, target_type, target_id,
			 ip, user_agent, request_id, summary, metadata_json, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?);`

	if _, err := s.db.ExecContext(ctx, q,
		util.NewID(),
		e.ActorType,
		nullable(e.ActorID),
		e.Action,
		nullable(e.TargetType),
		nullable(e.TargetID),
		nullable(e.IP),
		nullable(e.UserAgent),
		nullable(e.RequestID),
		nullable(e.Summary),
		metadataJSON,
		s.clock.Now(),
	); err != nil {
		// 仅告警，不影响调用方。
		log.Printf("audit: 写入审计日志失败 action=%s: %v", e.Action, err)
	}
}

// encodeMetadata 脱敏后序列化为 JSON；无内容或失败时返回 SQL NULL。
func (s *Service) encodeMetadata(metadata map[string]any) any {
	if len(metadata) == 0 {
		return nil
	}
	// 脱敏是写库前的强制步骤：口令、令牌、cookie 等绝不进审计表。
	safe := util.Redact(metadata)
	raw, err := json.Marshal(safe)
	if err != nil {
		log.Printf("audit: metadata 序列化失败: %v", err)
		return nil
	}
	return string(raw)
}

// nullable 把空字符串转换为 SQL NULL，使可空列保持语义清晰（区分"空"与"未提供"）。
func nullable(s string) any {
	if s == "" {
		return nil
	}
	return s
}
