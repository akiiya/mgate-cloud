package auth

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"mgate-cloud/internal/util"
)

// ThrottleSettings 配置登录失败限流策略。
//
// 语义：同一来源 IP 在 FailureWindow 窗口内连续失败达到 MaxFailures 次即封禁；
// 封禁时长从 BaseBan 起按封禁等级翻倍升级（失败越多、封得越久），上限 MaxBan；
// 距上次失败超过 OffenseReset 后封禁等级衰减归零，避免共享/动态 IP 永久升级。
type ThrottleSettings struct {
	Enabled       bool
	MaxFailures   int
	FailureWindow time.Duration
	BaseBan       time.Duration
	MaxBan        time.Duration
	OffenseReset  time.Duration
}

// loginAttempt 是 login_attempts 表一行的内存表示。
type loginAttempt struct {
	failCount   int
	banLevel    int
	lastFailure sql.NullTime
	bannedUntil sql.NullTime
}

// LoginThrottleStore 封装 login_attempts 表的持久化操作。
type LoginThrottleStore struct {
	db    *sql.DB
	clock util.Clock
}

// NewLoginThrottleStore 构造登录限流存储。
func NewLoginThrottleStore(db *sql.DB, clock util.Clock) *LoginThrottleStore {
	return &LoginThrottleStore{db: db, clock: clock}
}

func (s *LoginThrottleStore) get(ctx context.Context, ip string) (loginAttempt, bool, error) {
	const q = `SELECT fail_count, ban_level, last_failure_at, banned_until
		FROM login_attempts WHERE ip = ?;`
	var a loginAttempt
	err := s.db.QueryRowContext(ctx, q, ip).Scan(&a.failCount, &a.banLevel, &a.lastFailure, &a.bannedUntil)
	if errors.Is(err, sql.ErrNoRows) {
		return loginAttempt{}, false, nil
	}
	if err != nil {
		return loginAttempt{}, false, fmt.Errorf("auth: 查询登录限流记录失败: %w", err)
	}
	return a, true, nil
}

func (s *LoginThrottleStore) upsert(ctx context.Context, ip string, a loginAttempt) error {
	const q = `
		INSERT INTO login_attempts (ip, fail_count, ban_level, last_failure_at, banned_until, updated_at)
		VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT(ip) DO UPDATE SET
			fail_count      = excluded.fail_count,
			ban_level       = excluded.ban_level,
			last_failure_at = excluded.last_failure_at,
			banned_until    = excluded.banned_until,
			updated_at      = excluded.updated_at;`
	if _, err := s.db.ExecContext(ctx, q, ip, a.failCount, a.banLevel, a.lastFailure, a.bannedUntil, s.clock.Now()); err != nil {
		return fmt.Errorf("auth: 写入登录限流记录失败: %w", err)
	}
	return nil
}

func (s *LoginThrottleStore) delete(ctx context.Context, ip string) error {
	if _, err := s.db.ExecContext(ctx, `DELETE FROM login_attempts WHERE ip = ?;`, ip); err != nil {
		return fmt.Errorf("auth: 删除登录限流记录失败: %w", err)
	}
	return nil
}

// LoginThrottle 实现按来源 IP 的登录失败限流与升级封禁。
type LoginThrottle struct {
	store    *LoginThrottleStore
	clock    util.Clock
	settings ThrottleSettings
}

// NewLoginThrottle 构造登录限流器。MaxFailures 会被强制不小于 1，避免误配成 0 导致首次失败即封禁。
func NewLoginThrottle(store *LoginThrottleStore, clock util.Clock, settings ThrottleSettings) *LoginThrottle {
	if settings.MaxFailures < 1 {
		settings.MaxFailures = 1
	}
	return &LoginThrottle{store: store, clock: clock, settings: settings}
}

// Allow 报告该 IP 当前是否允许尝试登录；处于封禁中时返回剩余封禁时长。
//
// 失败开放：限流自身查询出错时放行（DB 故障时整体已不可用，不应让限流把管理员彻底锁死）。
func (t *LoginThrottle) Allow(ctx context.Context, ip string) (allowed bool, retryAfter time.Duration) {
	if !t.settings.Enabled || ip == "" {
		return true, 0
	}
	a, ok, err := t.store.get(ctx, ip)
	if err != nil || !ok {
		return true, 0
	}
	now := t.clock.Now()
	if a.bannedUntil.Valid && a.bannedUntil.Time.After(now) {
		return false, a.bannedUntil.Time.Sub(now)
	}
	return true, 0
}

// RecordFailure 记录一次登录失败；若因此触发封禁，返回 banned=true 与本次封禁时长。
func (t *LoginThrottle) RecordFailure(ctx context.Context, ip string) (banned bool, banFor time.Duration) {
	if !t.settings.Enabled || ip == "" {
		return false, 0
	}
	a, _, err := t.store.get(ctx, ip)
	if err != nil {
		return false, 0 // 失败开放
	}
	now := t.clock.Now()

	if a.lastFailure.Valid {
		gap := now.Sub(a.lastFailure.Time)
		// 距上次失败超过窗口：重置连续失败计数（窗口内才算"连续")。
		if gap > t.settings.FailureWindow {
			a.failCount = 0
		}
		// 长时间无失败：衰减封禁等级，避免共享/动态 IP 被永久升级封禁。
		if gap > t.settings.OffenseReset {
			a.banLevel = 0
		}
	}

	a.failCount++
	a.lastFailure = sql.NullTime{Time: now, Valid: true}

	if a.failCount >= t.settings.MaxFailures {
		a.banLevel++
		banFor = t.banDuration(a.banLevel)
		a.bannedUntil = sql.NullTime{Time: now.Add(banFor), Valid: true}
		a.failCount = 0 // 重置窗口计数；若继续失败将以更高等级、更长时长再次封禁
		banned = true
	}

	// 写库失败不影响已计算的结果（仅影响持久化），保持登录主流程稳定。
	_ = t.store.upsert(ctx, ip, a)
	return banned, banFor
}

// RecordSuccess 在登录成功后清除该 IP 的失败/封禁记录。
func (t *LoginThrottle) RecordSuccess(ctx context.Context, ip string) {
	if !t.settings.Enabled || ip == "" {
		return
	}
	_ = t.store.delete(ctx, ip)
}

// banDuration 计算第 level 次封禁的时长：BaseBan * 2^(level-1)，封顶 MaxBan。
func (t *LoginThrottle) banDuration(level int) time.Duration {
	if level < 1 {
		level = 1
	}
	d := t.settings.BaseBan
	for i := 1; i < level; i++ {
		d *= 2
		if d >= t.settings.MaxBan {
			return t.settings.MaxBan
		}
	}
	if d > t.settings.MaxBan {
		return t.settings.MaxBan
	}
	return d
}
