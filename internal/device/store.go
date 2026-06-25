// Package device 实现设备身份闭环的业务与持久化：
// 设备 CRUD、一次性设备码、enroll 与长期凭证。
//
// 分层：handler → device.Service → 各 Store → db。本包不含任何设备控制能力。
package device

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"mgate-cloud/internal/model"
)

// querier 抽象 *sql.DB 与 *sql.Tx 的共同方法集。
//
// 让 Store 方法既能在普通连接上执行，也能在事务（enroll）中执行，
// 而无需为事务版本重复一套 SQL。
type querier interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

// errDeviceNotFound 为包内错误，由 Service 映射为对外的 api.ErrDeviceNotFound。
var errDeviceNotFound = errors.New("device: 设备不存在")

// DeviceStore 封装 devices 表的持久化操作。
type DeviceStore struct{ db *sql.DB }

// NewDeviceStore 构造设备存储。
func NewDeviceStore(db *sql.DB) *DeviceStore { return &DeviceStore{db: db} }

// deviceColumns 是设备表的统一列序，供 SELECT 与 Scan 对齐，避免错位。
const deviceColumns = `id, name, remark, status, agent_version, mgate_version,
	device_model, hostname, firmware_info, last_seen_at, last_enrolled_at,
	last_ws_connected_at, last_ws_disconnected_at, capabilities_json,
	last_pull_at, last_pull_status_at, created_at, updated_at`

// Insert 写入一台新设备。
func (s *DeviceStore) Insert(ctx context.Context, q querier, d model.Device) error {
	const sqlStr = `
		INSERT INTO devices (id, name, remark, status, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?);`
	if _, err := q.ExecContext(ctx, sqlStr, d.ID, d.Name, d.Remark, d.Status, d.CreatedAt, d.UpdatedAt); err != nil {
		return fmt.Errorf("device: 创建设备失败: %w", err)
	}
	return nil
}

// List 返回设备列表（不含已软删除），按创建时间倒序。
func (s *DeviceStore) List(ctx context.Context, q querier) ([]model.Device, error) {
	sqlStr := `SELECT ` + deviceColumns + ` FROM devices WHERE status != ? ORDER BY created_at DESC;`
	rows, err := q.QueryContext(ctx, sqlStr, model.DeviceStatusDeleted)
	if err != nil {
		return nil, fmt.Errorf("device: 查询设备列表失败: %w", err)
	}
	defer rows.Close()

	var devices []model.Device
	for rows.Next() {
		d, err := scanDevice(rows)
		if err != nil {
			return nil, err
		}
		devices = append(devices, d)
	}
	return devices, rows.Err()
}

// GetByID 按 id 查询设备；不存在返回 errDeviceNotFound。
func (s *DeviceStore) GetByID(ctx context.Context, q querier, id string) (model.Device, error) {
	sqlStr := `SELECT ` + deviceColumns + ` FROM devices WHERE id = ?;`
	d, err := scanDevice(q.QueryRowContext(ctx, sqlStr, id))
	if errors.Is(err, sql.ErrNoRows) {
		return model.Device{}, errDeviceNotFound
	}
	if err != nil {
		return model.Device{}, fmt.Errorf("device: 查询设备失败: %w", err)
	}
	return d, nil
}

// UpdateStatus 仅更新设备状态与 updated_at。
func (s *DeviceStore) UpdateStatus(ctx context.Context, q querier, id, status string, now time.Time) error {
	const sqlStr = `UPDATE devices SET status = ?, updated_at = ? WHERE id = ?;`
	if _, err := q.ExecContext(ctx, sqlStr, status, now, id); err != nil {
		return fmt.Errorf("device: 更新设备状态失败: %w", err)
	}
	return nil
}

// EnrollInfo 承载 enroll 时由 agent 上报的设备自述信息。
type EnrollInfo struct {
	AgentVersion string
	MgateVersion string
	DeviceModel  string
	Hostname     string
	FirmwareInfo string
}

// ApplyEnroll 在 enroll 成功时把设备置为 enabled 并写入自述信息与绑定时间。
func (s *DeviceStore) ApplyEnroll(ctx context.Context, q querier, id string, info EnrollInfo, now time.Time) error {
	const sqlStr = `
		UPDATE devices SET
			status = ?, agent_version = ?, mgate_version = ?, device_model = ?,
			hostname = ?, firmware_info = ?, last_enrolled_at = ?, updated_at = ?
		WHERE id = ?;`
	if _, err := q.ExecContext(ctx, sqlStr,
		model.DeviceStatusEnabled,
		nullable(info.AgentVersion), nullable(info.MgateVersion), nullable(info.DeviceModel),
		nullable(info.Hostname), nullable(info.FirmwareInfo),
		now, now, id,
	); err != nil {
		return fmt.Errorf("device: 写入 enroll 结果失败: %w", err)
	}
	return nil
}

// TouchLastSeen 更新设备的最近活跃时间（心跳/状态上报时调用）。
func (s *DeviceStore) TouchLastSeen(ctx context.Context, q querier, id string, now time.Time) error {
	const sqlStr = `UPDATE devices SET last_seen_at = ?, updated_at = ? WHERE id = ?;`
	if _, err := q.ExecContext(ctx, sqlStr, now, now, id); err != nil {
		return fmt.Errorf("device: 更新 last_seen_at 失败: %w", err)
	}
	return nil
}

// SetWSConnected 记录 WebSocket 建连时间点（同时刷新 last_seen_at）。
func (s *DeviceStore) SetWSConnected(ctx context.Context, q querier, id string, now time.Time) error {
	const sqlStr = `UPDATE devices SET last_ws_connected_at = ?, last_seen_at = ?, updated_at = ? WHERE id = ?;`
	if _, err := q.ExecContext(ctx, sqlStr, now, now, now, id); err != nil {
		return fmt.Errorf("device: 记录 ws 连接时间失败: %w", err)
	}
	return nil
}

// SetWSDisconnected 记录 WebSocket 断开时间点。
func (s *DeviceStore) SetWSDisconnected(ctx context.Context, q querier, id string, now time.Time) error {
	const sqlStr = `UPDATE devices SET last_ws_disconnected_at = ?, updated_at = ? WHERE id = ?;`
	if _, err := q.ExecContext(ctx, sqlStr, now, now, id); err != nil {
		return fmt.Errorf("device: 记录 ws 断开时间失败: %w", err)
	}
	return nil
}

// SetLastPull 记录最近一次 HTTPS Pull 联系时间（同时刷新 last_seen_at）。
func (s *DeviceStore) SetLastPull(ctx context.Context, q querier, id string, now time.Time) error {
	const sqlStr = `UPDATE devices SET last_pull_at = ?, last_seen_at = ?, updated_at = ? WHERE id = ?;`
	if _, err := q.ExecContext(ctx, sqlStr, now, now, now, id); err != nil {
		return fmt.Errorf("device: 记录 last_pull_at 失败: %w", err)
	}
	return nil
}

// SetLastPullStatus 记录最近一次通过 Pull 上报状态的时间。
func (s *DeviceStore) SetLastPullStatus(ctx context.Context, q querier, id string, now time.Time) error {
	const sqlStr = `UPDATE devices SET last_pull_status_at = ?, updated_at = ? WHERE id = ?;`
	if _, err := q.ExecContext(ctx, sqlStr, now, now, id); err != nil {
		return fmt.Errorf("device: 记录 last_pull_status_at 失败: %w", err)
	}
	return nil
}

// ApplyHello 依据 agent.hello 更新设备自述信息与能力声明，并刷新活跃时间。
func (s *DeviceStore) ApplyHello(ctx context.Context, q querier, id string, info EnrollInfo, capabilitiesJSON string, now time.Time) error {
	const sqlStr = `
		UPDATE devices SET
			agent_version = ?, mgate_version = ?, device_model = ?,
			hostname = ?, firmware_info = ?, capabilities_json = ?,
			last_seen_at = ?, updated_at = ?
		WHERE id = ?;`
	if _, err := q.ExecContext(ctx, sqlStr,
		nullable(info.AgentVersion), nullable(info.MgateVersion), nullable(info.DeviceModel),
		nullable(info.Hostname), nullable(info.FirmwareInfo), nullable(capabilitiesJSON),
		now, now, id,
	); err != nil {
		return fmt.Errorf("device: 应用 hello 失败: %w", err)
	}
	return nil
}

// rowScanner 抽象 *sql.Row 与 *sql.Rows 的 Scan，使扫描逻辑可复用。
type rowScanner interface {
	Scan(dest ...any) error
}

// scanDevice 按 deviceColumns 顺序扫描一行设备。
func scanDevice(row rowScanner) (model.Device, error) {
	var d model.Device
	err := row.Scan(
		&d.ID, &d.Name, &d.Remark, &d.Status, &d.AgentVersion, &d.MgateVersion,
		&d.DeviceModel, &d.Hostname, &d.FirmwareInfo, &d.LastSeenAt, &d.LastEnrolledAt,
		&d.LastWSConnectedAt, &d.LastWSDisconnectedAt, &d.CapabilitiesJSON,
		&d.LastPullAt, &d.LastPullStatusAt,
		&d.CreatedAt, &d.UpdatedAt,
	)
	return d, err
}

// nullable 把空字符串转为 SQL NULL，保持可空列语义清晰。
func nullable(s string) any {
	if s == "" {
		return nil
	}
	return s
}
