-- 003_devices.sql
-- Phase 2：设备身份模型——设备、长期凭证、一次性设备码。
-- 本阶段只做"身份"与"绑定"，不涉及任何设备控制能力。

-- 设备表。
-- status 生命周期：pending（已创建未绑定）→ enabled（已绑定可连接）
--                 ⇄ disabled（已禁用，拒绝 agent 认证）；deleted 预留软删除。
CREATE TABLE devices (
    id               TEXT PRIMARY KEY,
    name             TEXT NOT NULL,
    remark           TEXT,
    status           TEXT NOT NULL DEFAULT 'pending',
    -- 以下设备自述信息在 enroll 时由 agent 上报填充。
    agent_version    TEXT,
    mgate_version    TEXT,
    device_model     TEXT,
    hostname         TEXT,
    firmware_info    TEXT,
    last_seen_at     DATETIME, -- 预留：Phase 2 不更新（无连接层）
    last_enrolled_at DATETIME,
    created_at       DATETIME NOT NULL,
    updated_at       DATETIME NOT NULL
);

-- 设备长期凭证表。
-- token 明文只在 enroll 成功响应里返回一次，数据库仅保存其哈希。
-- 禁用设备不删除凭证；rotate 能力为后续阶段预留（rotated_at/revoked_at 字段）。
CREATE TABLE device_credentials (
    id         TEXT PRIMARY KEY,
    device_id  TEXT NOT NULL,
    token_hash TEXT NOT NULL UNIQUE,
    status     TEXT NOT NULL DEFAULT 'active', -- active / revoked
    created_at DATETIME NOT NULL,
    rotated_at DATETIME,
    revoked_at DATETIME,
    FOREIGN KEY (device_id) REFERENCES devices(id)
);

-- 一次性设备码表。
-- 数据库只保存 pairing token 的哈希（code_hash）。设备码本体（含明文 pairing token
-- 与签名）只在生成时一次性展示，绝不落库。used_at 非空表示已被消费，不可重复使用。
CREATE TABLE device_pairing_codes (
    id                  TEXT PRIMARY KEY,
    device_id           TEXT NOT NULL,
    code_hash           TEXT NOT NULL UNIQUE,
    gateway_url         TEXT NOT NULL,
    expires_at          DATETIME NOT NULL,
    used_at             DATETIME,
    created_by_admin_id TEXT NOT NULL,
    created_at          DATETIME NOT NULL,
    FOREIGN KEY (device_id) REFERENCES devices(id),
    FOREIGN KEY (created_by_admin_id) REFERENCES admins(id)
);

-- 索引：列表按状态/时间浏览，凭证与设备码按设备维度查询，设备码按过期清理。
CREATE INDEX idx_devices_status ON devices(status);
CREATE INDEX idx_devices_created_at ON devices(created_at);
CREATE INDEX idx_device_credentials_device_id ON device_credentials(device_id);
CREATE INDEX idx_pairing_codes_device_id ON device_pairing_codes(device_id);
CREATE INDEX idx_pairing_codes_expires_at ON device_pairing_codes(expires_at);
