package model

import "time"

// PairingCode 对应 device_pairing_codes 表的一行。
//
// 数据库仅保存 pairing token 的哈希（CodeHash）。设备码本体（含明文 token 与签名）
// 只在生成时一次性展示给管理员，绝不落库。
type PairingCode struct {
	ID               string
	DeviceID         string
	CodeHash         string
	GatewayURL       string
	ExpiresAt        time.Time
	UsedAt           *time.Time
	CreatedByAdminID string
	CreatedAt        time.Time
}

// IsUsed 报告设备码是否已被消费。
func (p PairingCode) IsUsed() bool { return p.UsedAt != nil }

// IsExpired 报告设备码在给定时刻是否已过期。
func (p PairingCode) IsExpired(now time.Time) bool { return !now.Before(p.ExpiresAt) }
