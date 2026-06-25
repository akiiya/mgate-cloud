-- 004_agent_connections_status.sql
-- Phase 3：Agent WebSocket 连接元数据与设备状态上报。
-- 本阶段只做"连接"与"状态"，不涉及任何命令下发或设备控制能力。

-- devices 增补 WebSocket 连接时间与能力声明。
-- 说明：online（在线）是进程内连接的瞬时状态，不落库；落库的是连接/断开时间点。
-- capabilities_json 仅作记录，本阶段【不】据此下发任何命令。
ALTER TABLE devices ADD COLUMN last_ws_connected_at DATETIME;
ALTER TABLE devices ADD COLUMN last_ws_disconnected_at DATETIME;
ALTER TABLE devices ADD COLUMN capabilities_json TEXT;

-- 设备最新状态（每设备一行，覆盖更新）。
-- source 记录来源（如 agent.status），received_at 为 cloud 收到时间，reported_at 为 agent 声称的时间。
CREATE TABLE device_latest_status (
    device_id   TEXT PRIMARY KEY,
    status_json TEXT NOT NULL,
    reported_at DATETIME NOT NULL,
    received_at DATETIME NOT NULL,
    source      TEXT NOT NULL,
    FOREIGN KEY (device_id) REFERENCES devices(id)
);

-- 设备状态历史快照（追加写入），用于回溯。
CREATE TABLE device_status_snapshots (
    id          TEXT PRIMARY KEY,
    device_id   TEXT NOT NULL,
    status_json TEXT NOT NULL,
    reported_at DATETIME NOT NULL,
    received_at DATETIME NOT NULL,
    source      TEXT NOT NULL,
    FOREIGN KEY (device_id) REFERENCES devices(id)
);

-- 按设备 + 时间倒序查询快照。
CREATE INDEX idx_device_status_snapshots_device_time
    ON device_status_snapshots(device_id, received_at DESC);
