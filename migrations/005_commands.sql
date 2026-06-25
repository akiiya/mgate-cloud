-- 005_commands.sql
-- Phase 4：白名单命令队列与命令结果。
--
-- 边界：cloud 只负责"校验白名单 action → 落库 command → 经 WS 投递 JSON → 保存结果"。
-- cloud 不执行命令、不拼接 shell、不下发 raw command；真正调用 mgate.sh 是 agent 的职责。

CREATE TABLE commands (
    id                  TEXT PRIMARY KEY,
    device_id           TEXT NOT NULL,
    action              TEXT NOT NULL,                 -- 必须来自白名单
    params_json         TEXT NOT NULL DEFAULT '{}',    -- 经严格校验后的规范化参数
    status              TEXT NOT NULL DEFAULT 'pending',
    created_by_admin_id TEXT NOT NULL,
    idempotency_key     TEXT,                          -- 预留：幂等创建
    priority            INTEGER NOT NULL DEFAULT 100,  -- 预留：调度优先级
    timeout_sec         INTEGER NOT NULL DEFAULT 60,
    attempts            INTEGER NOT NULL DEFAULT 0,    -- 已投递次数
    max_attempts        INTEGER NOT NULL DEFAULT 1,    -- 预留：重试
    leased_by           TEXT,                          -- 预留：投递租约持有者
    lease_until         DATETIME,
    sent_at             DATETIME,
    acked_at            DATETIME,
    started_at          DATETIME,
    finished_at         DATETIME,
    expires_at          DATETIME,                      -- pending 超过此刻判 expired
    created_at          DATETIME NOT NULL,
    updated_at          DATETIME NOT NULL,
    FOREIGN KEY (device_id) REFERENCES devices(id),
    FOREIGN KEY (created_by_admin_id) REFERENCES admins(id)
);

-- 命令结果（每命令至多一条，command_id 唯一保证幂等）。
CREATE TABLE command_results (
    id            TEXT PRIMARY KEY,
    command_id    TEXT NOT NULL UNIQUE,
    device_id     TEXT NOT NULL,
    status        TEXT NOT NULL,        -- succeeded / failed / timeout
    exit_code     INTEGER,
    stdout        TEXT,
    stderr        TEXT,
    result_json   TEXT,
    error_message TEXT,
    truncated     INTEGER NOT NULL DEFAULT 0,  -- 任一字段被截断则置 1
    started_at    DATETIME,
    finished_at   DATETIME,
    received_at   DATETIME NOT NULL,
    FOREIGN KEY (command_id) REFERENCES commands(id),
    FOREIGN KEY (device_id) REFERENCES devices(id)
);

CREATE INDEX idx_commands_device_status ON commands(device_id, status);
CREATE INDEX idx_commands_status_created_at ON commands(status, created_at);
CREATE INDEX idx_commands_expires_at ON commands(expires_at);
CREATE INDEX idx_command_results_device_id ON command_results(device_id);
CREATE INDEX idx_command_results_received_at ON command_results(received_at);
