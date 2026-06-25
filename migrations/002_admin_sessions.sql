-- 002_admin_sessions.sql
-- 管理员会话表：服务端持久化会话，支持登出与吊销。

CREATE TABLE admin_sessions (
    id                 TEXT PRIMARY KEY,
    admin_id           TEXT NOT NULL,
    -- 仅存储会话令牌的哈希。原始令牌只在登录响应的 cookie 中下发一次，
    -- 数据库泄露时也无法据此伪造有效会话。
    session_token_hash TEXT NOT NULL UNIQUE,
    user_agent         TEXT,
    ip                 TEXT,
    expires_at         DATETIME NOT NULL,
    created_at         DATETIME NOT NULL,
    -- revoked_at 非空表示会话已被主动吊销（登出），用于软删除式失效。
    revoked_at         DATETIME,
    FOREIGN KEY (admin_id) REFERENCES admins(id)
);

-- 每次请求都需按令牌哈希定位会话，这是最高频查询，必须建索引。
CREATE INDEX idx_admin_sessions_token_hash ON admin_sessions(session_token_hash);
-- 按管理员维度查询/吊销其全部会话。
CREATE INDEX idx_admin_sessions_admin_id ON admin_sessions(admin_id);
