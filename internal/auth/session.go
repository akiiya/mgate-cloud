package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"mgate-cloud/internal/model"
	"mgate-cloud/internal/util"
)

// SessionCookieName 是会话 cookie 的名称。
const SessionCookieName = "mgate_session"

// sessionTokenBytes 是会话令牌的随机字节数（256 bit），足以抵御暴力枚举。
const sessionTokenBytes = 32

// generateToken 生成一个 URL 安全的高熵随机令牌。
//
// 同时用于会话令牌与 CSRF 令牌：均要求不可预测、不可枚举。
func generateToken() (string, error) {
	b := make([]byte, sessionTokenBytes)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("auth: 生成随机令牌失败: %w", err)
	}
	// RawURLEncoding：无填充、URL/cookie 安全。
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// hashToken 计算令牌的 SHA-256 十六进制摘要。
//
// 入库前一律哈希：数据库只见摘要，原始令牌仅存在于客户端 cookie。
// 令牌本身已是高熵随机串，无需加盐即可安全比对（不同于低熵口令）。
func hashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

// SessionStore 封装 admin_sessions 表的持久化操作。
//
// 作为会话数据的唯一访问入口，把 SQL 细节收敛于此，上层只面对领域语义。
type SessionStore struct {
	db    *sql.DB
	clock util.Clock
}

// NewSessionStore 构造会话存储。
func NewSessionStore(db *sql.DB, clock util.Clock) *SessionStore {
	return &SessionStore{db: db, clock: clock}
}

// Create 为指定管理员创建一个新会话，返回应写入 cookie 的"原始令牌"。
//
// 关键设计：原始令牌只在此处返回一次给调用方下发，绝不持久化；
// 数据库仅保存其哈希。调用方负责把返回的原始令牌写入 cookie。
func (s *SessionStore) Create(ctx context.Context, adminID, userAgent, ip string, ttl time.Duration) (rawToken string, err error) {
	token, err := generateToken()
	if err != nil {
		return "", err
	}

	now := s.clock.Now()
	session := model.AdminSession{
		ID:               util.NewID(),
		AdminID:          adminID,
		SessionTokenHash: hashToken(token),
		UserAgent:        userAgent,
		IP:               ip,
		ExpiresAt:        now.Add(ttl),
		CreatedAt:        now,
	}

	const q = `
		INSERT INTO admin_sessions
			(id, admin_id, session_token_hash, user_agent, ip, expires_at, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?);`
	if _, err := s.db.ExecContext(ctx, q,
		session.ID, session.AdminID, session.SessionTokenHash,
		session.UserAgent, session.IP, session.ExpiresAt, session.CreatedAt,
	); err != nil {
		return "", fmt.Errorf("auth: 写入会话失败: %w", err)
	}
	return token, nil
}

// ErrSessionNotFound 表示按令牌找不到有效会话。
var ErrSessionNotFound = errors.New("auth: 会话不存在或已失效")

// FindActiveByToken 根据原始令牌查找仍然有效（未吊销、未过期）的会话。
//
// 入参是原始令牌，函数内部自行哈希后查库，调用方无需关心存储细节。
func (s *SessionStore) FindActiveByToken(ctx context.Context, rawToken string) (model.AdminSession, error) {
	const q = `
		SELECT id, admin_id, session_token_hash, user_agent, ip, expires_at, created_at, revoked_at
		FROM admin_sessions
		WHERE session_token_hash = ?;`

	var sess model.AdminSession
	err := s.db.QueryRowContext(ctx, q, hashToken(rawToken)).Scan(
		&sess.ID, &sess.AdminID, &sess.SessionTokenHash,
		&sess.UserAgent, &sess.IP, &sess.ExpiresAt, &sess.CreatedAt, &sess.RevokedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return model.AdminSession{}, ErrSessionNotFound
	}
	if err != nil {
		return model.AdminSession{}, fmt.Errorf("auth: 查询会话失败: %w", err)
	}

	if !sess.IsActive(s.clock.Now()) {
		return model.AdminSession{}, ErrSessionNotFound
	}
	return sess, nil
}

// RevokeByToken 吊销与原始令牌对应的会话（登出）。
//
// 采用软删除（写 revoked_at）而非物理删除，保留会话历史以备审计。
// 即便令牌不存在也返回 nil：登出是幂等操作，重复登出不应报错。
func (s *SessionStore) RevokeByToken(ctx context.Context, rawToken string) error {
	const q = `
		UPDATE admin_sessions
		SET revoked_at = ?
		WHERE session_token_hash = ? AND revoked_at IS NULL;`
	if _, err := s.db.ExecContext(ctx, q, s.clock.Now(), hashToken(rawToken)); err != nil {
		return fmt.Errorf("auth: 吊销会话失败: %w", err)
	}
	return nil
}
