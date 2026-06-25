package device

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"mgate-cloud/internal/util"
)

// StatusRecord 是一条设备状态记录的入库形态，供最新状态与历史快照共用。
type StatusRecord struct {
	DeviceID   string
	StatusJSON string
	ReportedAt time.Time
	ReceivedAt time.Time
	Source     string
}

// LatestStatus 是某设备的最新状态快照（用于详情展示）。
type LatestStatus struct {
	StatusJSON string
	ReportedAt time.Time
	ReceivedAt time.Time
	Source     string
}

// StatusStore 封装 device_latest_status 与 device_status_snapshots 两表。
type StatusStore struct{ db *sql.DB }

// NewStatusStore 构造状态存储。
func NewStatusStore(db *sql.DB) *StatusStore { return &StatusStore{db: db} }

// UpsertLatest 覆盖写入设备最新状态（每设备一行）。
func (s *StatusStore) UpsertLatest(ctx context.Context, q querier, ls StatusRecord) error {
	const sqlStr = `
		INSERT INTO device_latest_status (device_id, status_json, reported_at, received_at, source)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(device_id) DO UPDATE SET
			status_json = excluded.status_json,
			reported_at = excluded.reported_at,
			received_at = excluded.received_at,
			source      = excluded.source;`
	if _, err := q.ExecContext(ctx, sqlStr, ls.DeviceID, ls.StatusJSON, ls.ReportedAt, ls.ReceivedAt, ls.Source); err != nil {
		return fmt.Errorf("device: 写入最新状态失败: %w", err)
	}
	return nil
}

// InsertSnapshot 追加一条状态历史快照。
func (s *StatusStore) InsertSnapshot(ctx context.Context, q querier, ls StatusRecord) error {
	const sqlStr = `
		INSERT INTO device_status_snapshots (id, device_id, status_json, reported_at, received_at, source)
		VALUES (?, ?, ?, ?, ?, ?);`
	if _, err := q.ExecContext(ctx, sqlStr, util.NewID(), ls.DeviceID, ls.StatusJSON, ls.ReportedAt, ls.ReceivedAt, ls.Source); err != nil {
		return fmt.Errorf("device: 写入状态快照失败: %w", err)
	}
	return nil
}

// GetLatest 读取设备最新状态；found=false 表示尚无状态上报。
func (s *StatusStore) GetLatest(ctx context.Context, q querier, deviceID string) (LatestStatus, bool, error) {
	const sqlStr = `SELECT status_json, reported_at, received_at, source FROM device_latest_status WHERE device_id = ?;`
	var ls LatestStatus
	err := q.QueryRowContext(ctx, sqlStr, deviceID).Scan(&ls.StatusJSON, &ls.ReportedAt, &ls.ReceivedAt, &ls.Source)
	if errors.Is(err, sql.ErrNoRows) {
		return LatestStatus{}, false, nil
	}
	if err != nil {
		return LatestStatus{}, false, fmt.Errorf("device: 查询最新状态失败: %w", err)
	}
	return ls, true, nil
}
