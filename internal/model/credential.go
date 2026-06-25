package model

import "time"

// 设备凭证状态枚举。
const (
	CredentialStatusActive  = "active"
	CredentialStatusRevoked = "revoked"
)

// DeviceCredential 对应 device_credentials 表的一行。
//
// 结构体只持有 TokenHash；设备长期令牌的明文仅在 enroll 成功响应里返回一次，
// 之后任何地方都无法再取回明文，从类型层面杜绝明文令牌被误用。
type DeviceCredential struct {
	ID        string
	DeviceID  string
	TokenHash string
	Status    string
	CreatedAt time.Time
	RotatedAt *time.Time
	RevokedAt *time.Time
}

// IsActive 报告凭证是否有效（状态为 active 且未吊销）。
func (c DeviceCredential) IsActive() bool {
	return c.Status == CredentialStatusActive && c.RevokedAt == nil
}
