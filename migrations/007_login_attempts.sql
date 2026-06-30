-- 007_login_attempts.sql
-- 管理员登录失败限流：按来源 IP 记录连续失败与封禁状态，
-- 实现「多次失败封禁、失败越多封禁越久」的在线暴力破解防护。
-- 持久化于库中，使重启不会清空封禁状态（攻击者无法通过触发重启绕过）。

CREATE TABLE login_attempts (
    ip              TEXT PRIMARY KEY,            -- 来源 IP（按可信代理策略解析）
    fail_count      INTEGER NOT NULL DEFAULT 0,  -- 当前窗口内的连续失败次数
    ban_level       INTEGER NOT NULL DEFAULT 0,  -- 历史封禁次数，用于升级封禁时长
    last_failure_at DATETIME,                    -- 最近一次失败时间（窗口与等级衰减判定）
    banned_until    DATETIME,                    -- 封禁截止时间；为空或已过去表示未封禁
    updated_at      DATETIME NOT NULL
);

-- 便于按封禁截止时间清理过期记录。
CREATE INDEX idx_login_attempts_banned_until ON login_attempts(banned_until);
