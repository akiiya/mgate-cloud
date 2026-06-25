package command

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"mgate-cloud/internal/model"
)

// querier 抽象 *sql.DB 与 *sql.Tx 的共同方法集（与 device 包同构）。
type querier interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

// errCommandNotFound 为包内错误，由 Service 映射为 api.ErrCommandNotFound。
var errCommandNotFound = errors.New("command: 命令不存在")

// Store 封装 commands 表的持久化。状态流转通过带 WHERE 守卫的条件更新实现，
// 返回受影响行数用于幂等与并发判断；状态机的"允许从哪些态转出"集中在此体现。
type Store struct{ db *sql.DB }

// NewStore 构造命令存储。
func NewStore(db *sql.DB) *Store { return &Store{db: db} }

const commandColumns = `id, device_id, action, params_json, status, created_by_admin_id,
	idempotency_key, priority, timeout_sec, attempts, max_attempts, leased_by, lease_until,
	sent_at, acked_at, started_at, finished_at, expires_at, last_error, created_at, updated_at`

// Insert 写入一条命令。
func (s *Store) Insert(ctx context.Context, q querier, c model.Command) error {
	const sqlStr = `
		INSERT INTO commands
			(id, device_id, action, params_json, status, created_by_admin_id,
			 priority, timeout_sec, attempts, max_attempts, expires_at, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?);`
	if _, err := q.ExecContext(ctx, sqlStr,
		c.ID, c.DeviceID, c.Action, c.ParamsJSON, c.Status, c.CreatedByAdminID,
		c.Priority, c.TimeoutSec, c.Attempts, c.MaxAttempts, c.ExpiresAt, c.CreatedAt, c.UpdatedAt,
	); err != nil {
		return fmt.Errorf("command: 创建命令失败: %w", err)
	}
	return nil
}

// GetByID 按 id 查询命令；不存在返回 errCommandNotFound。
func (s *Store) GetByID(ctx context.Context, q querier, id string) (model.Command, error) {
	sqlStr := `SELECT ` + commandColumns + ` FROM commands WHERE id = ?;`
	c, err := scanCommand(q.QueryRowContext(ctx, sqlStr, id))
	if errors.Is(err, sql.ErrNoRows) {
		return model.Command{}, errCommandNotFound
	}
	if err != nil {
		return model.Command{}, fmt.Errorf("command: 查询命令失败: %w", err)
	}
	return c, nil
}

// ListFilter 是命令列表的过滤条件。
type ListFilter struct {
	DeviceID string
	Status   string
	Limit    int
}

// ListItem 是列表项：命令 + 设备名。
type ListItem struct {
	Command    model.Command
	DeviceName string
}

// List 按过滤条件返回命令列表（关联设备名），按创建时间倒序。
func (s *Store) List(ctx context.Context, q querier, f ListFilter) ([]ListItem, error) {
	var where []string
	var args []any
	if f.DeviceID != "" {
		where = append(where, "c.device_id = ?")
		args = append(args, f.DeviceID)
	}
	if f.Status != "" {
		where = append(where, "c.status = ?")
		args = append(args, f.Status)
	}
	clause := ""
	if len(where) > 0 {
		clause = "WHERE " + strings.Join(where, " AND ")
	}
	limit := f.Limit
	if limit <= 0 || limit > 500 {
		limit = 100 // 默认与上限，避免一次拉取过多
	}

	sqlStr := `SELECT ` + prefixedCommandColumns("c") + `, COALESCE(d.name, '')
		FROM commands c LEFT JOIN devices d ON d.id = c.device_id
		` + clause + ` ORDER BY c.created_at DESC LIMIT ?;`
	args = append(args, limit)

	rows, err := q.QueryContext(ctx, sqlStr, args...)
	if err != nil {
		return nil, fmt.Errorf("command: 查询命令列表失败: %w", err)
	}
	defer rows.Close()

	var items []ListItem
	for rows.Next() {
		var it ListItem
		if err := scanCommandRow(rows, &it.Command, &it.DeviceName); err != nil {
			return nil, err
		}
		items = append(items, it)
	}
	return items, rows.Err()
}

// Lease 把"可领取的 pending"命令置为 leased（投递租约），并自增投递尝试次数。
//
// 可领取条件：status=pending 且（lease_until 为空或已过——后者表示重试退避已结束）。
// attempts 在领取时 +1：领取即视为一次投递尝试，达到 max_attempts 后由 reaper 判 timeout。
// 这是 WS 与 Pull 共用的原子闸门，保证同一命令不会被两个通道同时领取。
func (s *Store) Lease(ctx context.Context, q querier, id, leasedBy string, leaseUntil, now time.Time) (bool, error) {
	const sqlStr = `
		UPDATE commands SET status = ?, leased_by = ?, lease_until = ?, attempts = attempts + 1, updated_at = ?
		WHERE id = ? AND status = ? AND (lease_until IS NULL OR lease_until <= ?);`
	return affected(q.ExecContext(ctx, sqlStr,
		model.CommandStatusLeased, leasedBy, leaseUntil, now, id, model.CommandStatusPending, now))
}

// LeasePendingCommands 一次性领取设备的多条可领取 pending 命令（供 Pull 批量拉取）。
//
// 在调用方的事务内执行；配合 _txlock=immediate，并发 Pull/WS 不会重复领取同一命令。
func (s *Store) LeasePendingCommands(ctx context.Context, q querier, deviceID, leasedBy string, leaseUntil, now time.Time, limit int) ([]model.Command, error) {
	candidates, err := s.listPending(ctx, q, deviceID, now, limit)
	if err != nil {
		return nil, err
	}
	leased := make([]model.Command, 0, len(candidates))
	for _, c := range candidates {
		ok, err := s.Lease(ctx, q, c.ID, leasedBy, leaseUntil, now)
		if err != nil {
			return nil, err
		}
		if ok {
			c.Status = model.CommandStatusLeased
			c.Attempts++
			leased = append(leased, c)
		}
	}
	return leased, nil
}

// listPending 返回设备当前可领取的 pending 命令（按优先级、创建时间）。
func (s *Store) listPending(ctx context.Context, q querier, deviceID string, now time.Time, limit int) ([]model.Command, error) {
	if limit <= 0 {
		limit = 1
	}
	sqlStr := `SELECT ` + commandColumns + `
		FROM commands
		WHERE device_id = ? AND status = ? AND (lease_until IS NULL OR lease_until <= ?)
		ORDER BY priority ASC, created_at ASC LIMIT ?;`
	return s.queryCommands(ctx, q, sqlStr, deviceID, model.CommandStatusPending, now, limit)
}

// ReleaseLease 把 leased 命令退回 pending，并设置 retryAfter 作为下次可领取时间（退避）。
func (s *Store) ReleaseLease(ctx context.Context, q querier, id string, retryAfter, now time.Time) error {
	const sqlStr = `
		UPDATE commands SET status = ?, leased_by = NULL, lease_until = ?, updated_at = ?
		WHERE id = ? AND status = ?;`
	_, err := q.ExecContext(ctx, sqlStr, model.CommandStatusPending, retryAfter, now, id, model.CommandStatusLeased)
	return err
}

// MarkSent 把 leased 命令置为 sent（已投递）。attempts 已在领取时累加。
func (s *Store) MarkSent(ctx context.Context, q querier, id string, now time.Time) (bool, error) {
	const sqlStr = `
		UPDATE commands SET status = ?, sent_at = ?, updated_at = ?
		WHERE id = ? AND status = ?;`
	return affected(q.ExecContext(ctx, sqlStr,
		model.CommandStatusSent, now, now, id, model.CommandStatusLeased))
}

// RetryToPending 把卡住但仍可重试的命令退回 pending（设置退避时间），记录原因。
func (s *Store) RetryToPending(ctx context.Context, q querier, id, reason string, retryAfter, now time.Time) (bool, error) {
	const sqlStr = `
		UPDATE commands SET status = ?, leased_by = NULL, lease_until = ?, last_error = ?, updated_at = ?
		WHERE id = ? AND status IN (?, ?, ?, ?);`
	return affected(q.ExecContext(ctx, sqlStr,
		model.CommandStatusPending, retryAfter, reason, now, id,
		model.CommandStatusLeased, model.CommandStatusSent, model.CommandStatusAcked, model.CommandStatusRunning))
}

// MarkAcked 把 sent/leased 命令置为 acked（重复 ack 幂等）。
func (s *Store) MarkAcked(ctx context.Context, q querier, id string, now time.Time) (bool, error) {
	const sqlStr = `
		UPDATE commands SET status = ?, acked_at = COALESCE(acked_at, ?), updated_at = ?
		WHERE id = ? AND status IN (?, ?, ?);`
	return affected(q.ExecContext(ctx, sqlStr,
		model.CommandStatusAcked, now, now, id,
		model.CommandStatusSent, model.CommandStatusLeased, model.CommandStatusAcked))
}

// ApplyResultStatus 依据结果把非终态命令置为终态（succeeded/failed/timeout）。
// 仅当当前不是终态时才更新，避免覆盖已结束的命令。
func (s *Store) ApplyResultStatus(ctx context.Context, q querier, id, status string, startedAt, finishedAt *time.Time, now time.Time) (bool, error) {
	const sqlStr = `
		UPDATE commands SET status = ?, started_at = COALESCE(?, started_at), finished_at = ?, updated_at = ?
		WHERE id = ? AND status NOT IN (?, ?, ?, ?, ?);`
	return affected(q.ExecContext(ctx, sqlStr,
		status, startedAt, finishedAt, now, id,
		model.CommandStatusSucceeded, model.CommandStatusFailed,
		model.CommandStatusTimeout, model.CommandStatusCanceled, model.CommandStatusExpired))
}

// Cancel 把非终态命令置为 canceled，返回是否成功。
func (s *Store) Cancel(ctx context.Context, q querier, id string, now time.Time) (bool, error) {
	const sqlStr = `
		UPDATE commands SET status = ?, finished_at = ?, updated_at = ?
		WHERE id = ? AND status NOT IN (?, ?, ?, ?, ?);`
	return affected(q.ExecContext(ctx, sqlStr,
		model.CommandStatusCanceled, now, now, id,
		model.CommandStatusSucceeded, model.CommandStatusFailed,
		model.CommandStatusTimeout, model.CommandStatusCanceled, model.CommandStatusExpired))
}

// MarkTimeout 把卡住的命令（leased/sent/acked/running）置为 timeout。
func (s *Store) MarkTimeout(ctx context.Context, q querier, id string, now time.Time) (bool, error) {
	const sqlStr = `
		UPDATE commands SET status = ?, finished_at = ?, updated_at = ?
		WHERE id = ? AND status IN (?, ?, ?, ?);`
	return affected(q.ExecContext(ctx, sqlStr,
		model.CommandStatusTimeout, now, now, id,
		model.CommandStatusLeased, model.CommandStatusSent, model.CommandStatusAcked, model.CommandStatusRunning))
}

// MarkExpired 把超过 TTL 的 pending 命令置为 expired。
func (s *Store) MarkExpired(ctx context.Context, q querier, id string, now time.Time) (bool, error) {
	const sqlStr = `
		UPDATE commands SET status = ?, finished_at = ?, updated_at = ?
		WHERE id = ? AND status = ?;`
	return affected(q.ExecContext(ctx, sqlStr,
		model.CommandStatusExpired, now, now, id, model.CommandStatusPending))
}

// FindActive 返回所有"进行中"的命令（leased/sent/acked/running），供 reaper 扫描。
func (s *Store) FindActive(ctx context.Context, q querier) ([]model.Command, error) {
	sqlStr := `SELECT ` + commandColumns + ` FROM commands WHERE status IN (?, ?, ?, ?);`
	return s.queryCommands(ctx, q, sqlStr,
		model.CommandStatusLeased, model.CommandStatusSent, model.CommandStatusAcked, model.CommandStatusRunning)
}

// ListPendingForDevice 返回设备当前可领取的 pending 命令（供 WS 重连后投递）。
func (s *Store) ListPendingForDevice(ctx context.Context, q querier, deviceID string, now time.Time, limit int) ([]model.Command, error) {
	return s.listPending(ctx, q, deviceID, now, limit)
}

// FindExpiredPending 返回已过期的 pending 命令（expires_at <= now）。
func (s *Store) FindExpiredPending(ctx context.Context, q querier, now time.Time) ([]model.Command, error) {
	sqlStr := `SELECT ` + commandColumns + ` FROM commands WHERE status = ? AND expires_at IS NOT NULL AND expires_at <= ?;`
	return s.queryCommands(ctx, q, sqlStr, model.CommandStatusPending, now)
}

// queryCommands 执行返回多行命令的查询。
func (s *Store) queryCommands(ctx context.Context, q querier, sqlStr string, args ...any) ([]model.Command, error) {
	rows, err := q.QueryContext(ctx, sqlStr, args...)
	if err != nil {
		return nil, fmt.Errorf("command: 查询命令失败: %w", err)
	}
	defer rows.Close()
	var out []model.Command
	for rows.Next() {
		var c model.Command
		if err := scanCommandRow(rows, &c, nil); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// --- 扫描辅助 ---

type rowScanner interface{ Scan(dest ...any) error }

func prefixedCommandColumns(alias string) string {
	cols := strings.Split(commandColumns, ",")
	for i, c := range cols {
		cols[i] = alias + "." + strings.TrimSpace(c)
	}
	return strings.Join(cols, ", ")
}

// scanCommand 扫描单行命令（不含设备名）。
func scanCommand(row rowScanner) (model.Command, error) {
	var c model.Command
	err := scanCommandRow(row, &c, nil)
	return c, err
}

// scanCommandRow 把一行扫描进 Command；deviceName 非 nil 时额外扫描设备名列。
func scanCommandRow(row rowScanner, c *model.Command, deviceName *string) error {
	dest := []any{
		&c.ID, &c.DeviceID, &c.Action, &c.ParamsJSON, &c.Status, &c.CreatedByAdminID,
		&c.IdempotencyKey, &c.Priority, &c.TimeoutSec, &c.Attempts, &c.MaxAttempts, &c.LeasedBy, &c.LeaseUntil,
		&c.SentAt, &c.AckedAt, &c.StartedAt, &c.FinishedAt, &c.ExpiresAt, &c.LastError, &c.CreatedAt, &c.UpdatedAt,
	}
	if deviceName != nil {
		dest = append(dest, deviceName)
	}
	return row.Scan(dest...)
}

// affected 把 ExecContext 结果转换为"是否影响了至少一行"。
func affected(res sql.Result, err error) (bool, error) {
	if err != nil {
		return false, fmt.Errorf("command: 状态更新失败: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("command: 读取受影响行数失败: %w", err)
	}
	return n > 0, nil
}
