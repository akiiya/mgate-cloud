package command

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"mgate-cloud/internal/model"
	"mgate-cloud/internal/util"
)

// ResultStore 封装 command_results 表的持久化。
type ResultStore struct{ db *sql.DB }

// NewResultStore 构造结果存储。
func NewResultStore(db *sql.DB) *ResultStore { return &ResultStore{db: db} }

// InsertIfAbsent 写入命令结果；若该 command 已有结果则不插入（幂等），返回是否真正写入。
//
// command_results.command_id 唯一约束保证一条命令至多一条结果；
// 这里用 ON CONFLICT DO NOTHING，使 agent 重发 result 不会产生多条记录。
func (s *ResultStore) InsertIfAbsent(ctx context.Context, q querier, r model.CommandResult) (bool, error) {
	const sqlStr = `
		INSERT INTO command_results
			(id, command_id, device_id, status, exit_code, stdout, stderr, result_json,
			 error_message, truncated, started_at, finished_at, received_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(command_id) DO NOTHING;`
	res, err := q.ExecContext(ctx, sqlStr,
		util.NewID(), r.CommandID, r.DeviceID, r.Status, r.ExitCode, r.Stdout, r.Stderr, r.ResultJSON,
		r.ErrorMessage, boolToInt(r.Truncated), r.StartedAt, r.FinishedAt, r.ReceivedAt,
	)
	return affected(res, err)
}

// GetByCommandID 读取命令结果；found=false 表示尚无结果。
func (s *ResultStore) GetByCommandID(ctx context.Context, q querier, commandID string) (model.CommandResult, bool, error) {
	const sqlStr = `
		SELECT id, command_id, device_id, status, exit_code, stdout, stderr, result_json,
		       error_message, truncated, started_at, finished_at, received_at
		FROM command_results WHERE command_id = ?;`
	var r model.CommandResult
	var truncated int
	err := q.QueryRowContext(ctx, sqlStr, commandID).Scan(
		&r.ID, &r.CommandID, &r.DeviceID, &r.Status, &r.ExitCode, &r.Stdout, &r.Stderr, &r.ResultJSON,
		&r.ErrorMessage, &truncated, &r.StartedAt, &r.FinishedAt, &r.ReceivedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return model.CommandResult{}, false, nil
	}
	if err != nil {
		return model.CommandResult{}, false, fmt.Errorf("command: 查询命令结果失败: %w", err)
	}
	r.Truncated = truncated != 0
	return r, true, nil
}

// truncate 把字符串限制到 max 字节；超出则截断并返回 truncated=true。
//
// 按字节截断后修正到合法 UTF-8 边界，避免产生半个字符。
func truncate(s string, max int) (string, bool) {
	if len(s) <= max {
		return s, false
	}
	cut := s[:max]
	// 回退到完整 UTF-8 字符边界（最多回退 3 字节）。
	for i := 0; i < 3 && len(cut) > 0; i++ {
		if isUTF8Boundary(cut) {
			break
		}
		cut = cut[:len(cut)-1]
	}
	return cut, true
}

// isUTF8Boundary 判断字符串结尾是否为完整 UTF-8 序列。
func isUTF8Boundary(s string) bool {
	// 找到最后一个起始字节，校验其后续字节数是否完整。
	for i := len(s) - 1; i >= 0 && i > len(s)-4; i-- {
		b := s[i]
		if b < 0x80 { // ASCII，单字节
			return i == len(s)-1
		}
		if b >= 0xC0 { // 多字节序列起始
			need := 2
			switch {
			case b >= 0xF0:
				need = 4
			case b >= 0xE0:
				need = 3
			}
			return len(s)-i == need
		}
	}
	return false
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
