package device

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"mgate-cloud/internal/model"
)

// errCredentialNotFound 表示设备没有有效凭证。
var errCredentialNotFound = errors.New("device: 无有效凭证")

// CredentialStore 封装 device_credentials 表的持久化操作。
type CredentialStore struct{ db *sql.DB }

// NewCredentialStore 构造凭证存储。
func NewCredentialStore(db *sql.DB) *CredentialStore { return &CredentialStore{db: db} }

// Insert 写入一条设备凭证（仅哈希）。
func (s *CredentialStore) Insert(ctx context.Context, q querier, c model.DeviceCredential) error {
	const sqlStr = `
		INSERT INTO device_credentials (id, device_id, token_hash, status, created_at)
		VALUES (?, ?, ?, ?, ?);`
	if _, err := q.ExecContext(ctx, sqlStr, c.ID, c.DeviceID, c.TokenHash, c.Status, c.CreatedAt); err != nil {
		return fmt.Errorf("device: 写入设备凭证失败: %w", err)
	}
	return nil
}

// CountActiveByDevice 统计某设备的有效凭证数量。
//
// 用于"启用设备时判断应回到 pending 还是 enabled"：从未 enroll（无 active 凭证）
// 的设备启用后回到 pending，已绑定的回到 enabled。
func (s *CredentialStore) CountActiveByDevice(ctx context.Context, q querier, deviceID string) (int, error) {
	const sqlStr = `SELECT COUNT(*) FROM device_credentials WHERE device_id = ? AND status = ? AND revoked_at IS NULL;`
	var n int
	if err := q.QueryRowContext(ctx, sqlStr, deviceID, model.CredentialStatusActive).Scan(&n); err != nil {
		return 0, fmt.Errorf("device: 统计有效凭证失败: %w", err)
	}
	return n, nil
}

// FindActiveByDevice 返回设备最近一条有效凭证；无有效凭证返回 errCredentialNotFound。
//
// 用于 WebSocket 鉴权：取出存储的 token_hash 后由调用方做恒定时间比较。
func (s *CredentialStore) FindActiveByDevice(ctx context.Context, q querier, deviceID string) (model.DeviceCredential, error) {
	const sqlStr = `
		SELECT id, device_id, token_hash, status, created_at, rotated_at, revoked_at
		FROM device_credentials
		WHERE device_id = ? AND status = ? AND revoked_at IS NULL
		ORDER BY created_at DESC LIMIT 1;`
	var c model.DeviceCredential
	err := q.QueryRowContext(ctx, sqlStr, deviceID, model.CredentialStatusActive).Scan(
		&c.ID, &c.DeviceID, &c.TokenHash, &c.Status, &c.CreatedAt, &c.RotatedAt, &c.RevokedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return model.DeviceCredential{}, errCredentialNotFound
	}
	if err != nil {
		return model.DeviceCredential{}, fmt.Errorf("device: 查询有效凭证失败: %w", err)
	}
	return c, nil
}
