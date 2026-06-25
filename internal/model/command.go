package model

import "time"

// 命令状态枚举。
//
//	pending   已创建，待投递
//	leased    已被投递流程租用（短暂中间态）
//	sent      已通过 WS 投递给 agent
//	acked     agent 已确认接收
//	running   agent 执行中（预留，Phase 4 由 result 直接进入终态）
//	succeeded 执行成功（终态）
//	failed    执行失败 / 被 agent 拒绝（终态）
//	timeout   超时未完成（终态）
//	canceled  被管理员取消（终态）
//	expired   pending 超过 TTL（终态）
const (
	CommandStatusPending   = "pending"
	CommandStatusLeased    = "leased"
	CommandStatusSent      = "sent"
	CommandStatusAcked     = "acked"
	CommandStatusRunning   = "running"
	CommandStatusSucceeded = "succeeded"
	CommandStatusFailed    = "failed"
	CommandStatusTimeout   = "timeout"
	CommandStatusCanceled  = "canceled"
	CommandStatusExpired   = "expired"
)

// Command 对应 commands 表的一行。
type Command struct {
	ID               string
	DeviceID         string
	Action           string
	ParamsJSON       string
	Status           string
	CreatedByAdminID string
	IdempotencyKey   *string
	Priority         int
	TimeoutSec       int
	Attempts         int
	MaxAttempts      int
	LeasedBy         *string
	LeaseUntil       *time.Time
	SentAt           *time.Time
	AckedAt          *time.Time
	StartedAt        *time.Time
	FinishedAt       *time.Time
	ExpiresAt        *time.Time
	LastError        *string
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

// CommandResult 对应 command_results 表的一行。
type CommandResult struct {
	ID           string
	CommandID    string
	DeviceID     string
	Status       string
	ExitCode     *int
	Stdout       *string
	Stderr       *string
	ResultJSON   *string
	ErrorMessage *string
	Truncated    bool
	StartedAt    *time.Time
	FinishedAt   *time.Time
	ReceivedAt   time.Time
}
