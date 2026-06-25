package model

import "time"

// 设备状态枚举。
//
//	pending  管理员已创建，尚未绑定（可生成设备码）
//	enabled  已绑定，可连接
//	disabled 已禁用，拒绝 agent 认证（不可 enroll）
//	deleted  软删除（Phase 2 暂不提供删除入口，预留）
const (
	DeviceStatusPending  = "pending"
	DeviceStatusEnabled  = "enabled"
	DeviceStatusDisabled = "disabled"
	DeviceStatusDeleted  = "deleted"
)

// Device 对应 devices 表的一行。
//
// 可空的设备自述字段（enroll 前为空）用指针表达，区分"未上报"与"空字符串"。
type Device struct {
	ID             string
	Name           string
	Remark         *string
	Status         string
	AgentVersion   *string
	MgateVersion   *string
	DeviceModel    *string
	Hostname       *string
	FirmwareInfo   *string
	LastSeenAt     *time.Time
	LastEnrolledAt *time.Time
	// Phase 3：WebSocket 连接时间点与能力声明（capabilities 仅记录，不用于下发命令）。
	LastWSConnectedAt    *time.Time
	LastWSDisconnectedAt *time.Time
	CapabilitiesJSON     *string
	// Phase 5：HTTPS Pull 时间点（与 WS online 区分）。
	LastPullAt       *time.Time
	LastPullStatusAt *time.Time
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

// IsPending 报告设备是否处于待绑定状态。
func (d Device) IsPending() bool { return d.Status == DeviceStatusPending }

// IsDisabled 报告设备是否被禁用。
func (d Device) IsDisabled() bool { return d.Status == DeviceStatusDisabled }
