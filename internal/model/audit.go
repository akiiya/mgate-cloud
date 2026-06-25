package model

import "time"

// 行为主体类型。
const (
	ActorTypeAdmin  = "admin"
	ActorTypeSystem = "system"
	ActorTypeDevice = "device"
)

// 审计目标类型。
const (
	TargetTypeDevice  = "device"
	TargetTypePairing = "pairing_code"
	TargetTypeCommand = "command"
)

// 审计事件动作枚举。集中声明，保证写入与查询使用同一套稳定字符串。
const (
	ActionLoginSuccess          = "admin.login.success"
	ActionLoginFailed           = "admin.login.failed"
	ActionLogout                = "admin.logout"
	ActionBootstrapAdminCreated = "system.bootstrap_admin.created"
	ActionSetupCompleted        = "system.setup.completed"
	ActionUpdateChecked         = "system.update.checked"
	ActionUpdateApplied         = "system.update.applied"

	// Phase 2：设备身份相关事件
	ActionDeviceCreate      = "device.create"
	ActionDeviceDisable     = "device.disable"
	ActionDeviceEnable      = "device.enable"
	ActionPairingCodeCreate = "pairing_code.create"
	ActionDeviceEnrollOK    = "device.enroll.success"
	ActionDeviceEnrollFail  = "device.enroll.failed"

	// Phase 3：Agent WebSocket 连接与状态上报事件
	ActionDeviceWSAuthFailed = "device.ws.auth_failed"
	ActionDeviceWSConnect    = "device.ws.connect"
	ActionDeviceWSDisconnect = "device.ws.disconnect"
	ActionDeviceHello        = "device.hello"
	ActionDeviceStatus       = "device.status.reported"

	// Phase 4：命令队列事件
	ActionCommandCreate   = "command.create"
	ActionCommandDeliver  = "command.deliver"
	ActionCommandAck      = "command.ack"
	ActionCommandRejected = "command.rejected"
	ActionCommandResult   = "command.result"
	ActionCommandTimeout  = "command.timeout"
	ActionCommandExpired  = "command.expired"
	ActionCommandCancel   = "command.cancel"
	ActionCommandRetry    = "command.retry"

	// Phase 5：HTTPS Pull 兜底
	ActionDevicePull         = "device.pull"
	ActionDevicePullAuthFail = "device.pull.auth_failed"
	ActionDevicePullStatus   = "device.pull.status_reported"
)

// AuditLog 对应 audit_logs 表的一行。
//
// 可空字段统一用指针表达，区分"空字符串"与"未提供"。
type AuditLog struct {
	ID           string
	ActorType    string
	ActorID      *string
	Action       string
	TargetType   *string
	TargetID     *string
	IP           *string
	UserAgent    *string
	RequestID    *string
	Summary      *string
	MetadataJSON *string
	CreatedAt    time.Time
}
