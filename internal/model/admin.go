// Package model 定义领域实体的内存结构，与数据库表一一对应。
//
// 这里只放纯数据结构与少量与持久化无关的领域判断，不含 SQL 与业务流程。
package model

import "time"

// 管理员账户状态枚举。Phase 1 仅使用 enabled，disabled 为后续禁用账户预留。
const (
	AdminStatusEnabled  = "enabled"
	AdminStatusDisabled = "disabled"
)

// Admin 对应 admins 表的一行。
//
// 注意：结构体只持有 PasswordHash（哈希值），不存在明文口令字段，
// 从类型层面杜绝明文口令在内存中被误用或被序列化输出。
type Admin struct {
	ID           string
	Username     string
	PasswordHash string
	Status       string
	CreatedAt    time.Time
	UpdatedAt    time.Time
	// LastLoginAt 可能为空（从未登录），用指针表达"可空"语义。
	LastLoginAt *time.Time
}

// IsEnabled 判断账户是否处于可登录状态。
func (a Admin) IsEnabled() bool {
	return a.Status == AdminStatusEnabled
}

// AdminSession 对应 admin_sessions 表的一行。
//
// 同样地，结构体只保存令牌哈希，原始令牌仅存在于下发给客户端的 cookie 中。
type AdminSession struct {
	ID               string
	AdminID          string
	SessionTokenHash string
	UserAgent        string
	IP               string
	ExpiresAt        time.Time
	CreatedAt        time.Time
	RevokedAt        *time.Time
}

// IsActive 判断会话在给定时刻是否仍然有效（未吊销且未过期）。
func (s AdminSession) IsActive(now time.Time) bool {
	if s.RevokedAt != nil {
		return false
	}
	return now.Before(s.ExpiresAt)
}
