package auth

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"mgate-cloud/internal/api"
	"mgate-cloud/internal/model"
	"mgate-cloud/internal/util"
)

// AdminStore 封装 admins 表的持久化操作（repository 层）。
type AdminStore struct {
	db    *sql.DB
	clock util.Clock
}

// NewAdminStore 构造管理员存储。
func NewAdminStore(db *sql.DB, clock util.Clock) *AdminStore {
	return &AdminStore{db: db, clock: clock}
}

// errAdminNotFound 为内部错误，不直接对外（对外统一映射为凭据错误，防账户枚举）。
var errAdminNotFound = errors.New("auth: 管理员不存在")

// FindByUsername 按用户名查找管理员。
func (s *AdminStore) FindByUsername(ctx context.Context, username string) (model.Admin, error) {
	const q = `
		SELECT id, username, password_hash, status, created_at, updated_at, last_login_at
		FROM admins WHERE username = ?;`
	return s.scanAdmin(s.db.QueryRowContext(ctx, q, username))
}

// FindByID 按 id 查找管理员（会话解析时使用）。
func (s *AdminStore) FindByID(ctx context.Context, id string) (model.Admin, error) {
	const q = `
		SELECT id, username, password_hash, status, created_at, updated_at, last_login_at
		FROM admins WHERE id = ?;`
	return s.scanAdmin(s.db.QueryRowContext(ctx, q, id))
}

// scanAdmin 统一扫描单行管理员，集中处理"未找到"语义。
func (s *AdminStore) scanAdmin(row *sql.Row) (model.Admin, error) {
	var a model.Admin
	err := row.Scan(&a.ID, &a.Username, &a.PasswordHash, &a.Status, &a.CreatedAt, &a.UpdatedAt, &a.LastLoginAt)
	if errors.Is(err, sql.ErrNoRows) {
		return model.Admin{}, errAdminNotFound
	}
	if err != nil {
		return model.Admin{}, fmt.Errorf("auth: 查询管理员失败: %w", err)
	}
	return a, nil
}

// Count 返回管理员总数，用于判断是否需要 bootstrap。
func (s *AdminStore) Count(ctx context.Context) (int, error) {
	var n int
	if err := s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM admins;").Scan(&n); err != nil {
		return 0, fmt.Errorf("auth: 统计管理员数量失败: %w", err)
	}
	return n, nil
}

// Create 新建管理员，接受已哈希的口令。
func (s *AdminStore) Create(ctx context.Context, username, passwordHash string) (model.Admin, error) {
	now := s.clock.Now()
	a := model.Admin{
		ID:           util.NewID(),
		Username:     username,
		PasswordHash: passwordHash,
		Status:       model.AdminStatusEnabled,
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	const q = `
		INSERT INTO admins (id, username, password_hash, status, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?);`
	if _, err := s.db.ExecContext(ctx, q, a.ID, a.Username, a.PasswordHash, a.Status, a.CreatedAt, a.UpdatedAt); err != nil {
		return model.Admin{}, fmt.Errorf("auth: 创建管理员失败: %w", err)
	}
	return a, nil
}

// touchLastLogin 更新最近登录时间。失败不应阻断登录主流程，由调用方决定如何处理。
func (s *AdminStore) touchLastLogin(ctx context.Context, adminID string) error {
	now := s.clock.Now()
	const q = `UPDATE admins SET last_login_at = ?, updated_at = ? WHERE id = ?;`
	if _, err := s.db.ExecContext(ctx, q, now, now, adminID); err != nil {
		return fmt.Errorf("auth: 更新最近登录时间失败: %w", err)
	}
	return nil
}

// Service 编排认证业务：登录、登出、会话解析、bootstrap。
//
// 它把 AdminStore 与 SessionStore 组合起来，向 handler 暴露"领域动作"，
// 使 handler 保持瘦身、只做 HTTP 适配。
type Service struct {
	admins     *AdminStore
	sessions   *SessionStore
	sessionTTL time.Duration
}

// NewService 构造认证服务。
func NewService(admins *AdminStore, sessions *SessionStore, sessionTTL time.Duration) *Service {
	return &Service{admins: admins, sessions: sessions, sessionTTL: sessionTTL}
}

// Login 校验凭据并创建会话，成功时返回应下发到 cookie 的原始令牌与管理员信息。
//
// 安全要点：无论"用户名不存在"还是"口令错误"，一律返回 api.ErrInvalidCredentials，
// 不向外透露差异，杜绝账户枚举。
func (s *Service) Login(ctx context.Context, username, password, userAgent, ip string) (rawToken string, admin model.Admin, err error) {
	admin, err = s.admins.FindByUsername(ctx, username)
	if errors.Is(err, errAdminNotFound) {
		// 计时对齐：用户名不存在时也消耗一次等价的 bcrypt 时间，避免据响应耗时枚举有效用户名。
		DummyVerify(password)
		return "", model.Admin{}, api.ErrInvalidCredentials
	}
	if err != nil {
		return "", model.Admin{}, err
	}

	// 账户被禁用同样返回统一凭据错误，不暴露账户状态；同样做计时对齐。
	if !admin.IsEnabled() {
		DummyVerify(password)
		return "", model.Admin{}, api.ErrInvalidCredentials
	}

	if !VerifyPassword(admin.PasswordHash, password) {
		return "", model.Admin{}, api.ErrInvalidCredentials
	}

	rawToken, err = s.sessions.Create(ctx, admin.ID, userAgent, ip, s.sessionTTL)
	if err != nil {
		return "", model.Admin{}, err
	}

	// 最近登录时间属于非关键信息，更新失败不应导致登录失败，仅记录。
	if err := s.admins.touchLastLogin(ctx, admin.ID); err != nil {
		// 此处不向上抛，避免影响登录；交由上层日志体系记录即可。
		_ = err
	}

	return rawToken, admin, nil
}

// AdminCount 返回当前管理员数量，供启动期判断是否缺少管理员。
func (s *Service) AdminCount(ctx context.Context) (int, error) {
	return s.admins.Count(ctx)
}

// CreateInitialAdmin 在系统尚无管理员时，用预先计算的 bcrypt 哈希创建管理员（供 setup 使用）。
//
// 仅当当前无任何管理员时才创建，避免 setup 被重复触发产生多个账户。
func (s *Service) CreateInitialAdmin(ctx context.Context, username, passwordHash string) (model.Admin, error) {
	count, err := s.admins.Count(ctx)
	if err != nil {
		return model.Admin{}, err
	}
	if count > 0 {
		return model.Admin{}, fmt.Errorf("auth: 已存在管理员，拒绝重复初始化")
	}
	return s.admins.Create(ctx, username, passwordHash)
}

// ResolveSession 根据原始令牌解析出有效会话对应的管理员（供鉴权中间件使用）。
func (s *Service) ResolveSession(ctx context.Context, rawToken string) (model.Admin, error) {
	sess, err := s.sessions.FindActiveByToken(ctx, rawToken)
	if errors.Is(err, ErrSessionNotFound) {
		return model.Admin{}, api.ErrUnauthorized
	}
	if err != nil {
		return model.Admin{}, err
	}

	admin, err := s.admins.FindByID(ctx, sess.AdminID)
	if errors.Is(err, errAdminNotFound) || (err == nil && !admin.IsEnabled()) {
		// 会话指向的账户已不存在或被禁用：视为未授权。
		return model.Admin{}, api.ErrUnauthorized
	}
	if err != nil {
		return model.Admin{}, err
	}
	return admin, nil
}

// Logout 吊销原始令牌对应的会话。
func (s *Service) Logout(ctx context.Context, rawToken string) error {
	return s.sessions.RevokeByToken(ctx, rawToken)
}

// BootstrapAdmin 在系统尚无任何管理员时创建初始管理员。
//
// 返回 created 表示本次是否真正创建：已存在管理员时返回 (false, ...)，
// 保证重复启动不会重复创建，满足幂等。
func (s *Service) BootstrapAdmin(ctx context.Context, username, password string) (created bool, admin model.Admin, err error) {
	count, err := s.admins.Count(ctx)
	if err != nil {
		return false, model.Admin{}, err
	}
	if count > 0 {
		return false, model.Admin{}, nil
	}

	hash, err := HashPassword(password)
	if err != nil {
		return false, model.Admin{}, err
	}
	admin, err = s.admins.Create(ctx, username, hash)
	if err != nil {
		return false, model.Admin{}, err
	}
	return true, admin, nil
}
