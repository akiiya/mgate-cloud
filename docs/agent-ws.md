# Agent WebSocket 协议 🔌

本文描述 Phase 3 的 Agent WebSocket 通道：连接鉴权、消息信封、支持的消息类型、
在线状态判定与部署注意事项。

> 本文聚焦连接与状态消息（Phase 3）。Phase 4 起本通道额外承载**命令通道**
> （`command.deliver` / `command.ack` / `command.result`），其设计见 [commands.md](commands.md)。
>
> 当 agent 无法保持 WebSocket 时，可改用 **HTTPS Pull 兜底**（Phase 5）：见 [agent-pull.md](agent-pull.md)。
> 两通道共用命令 lease，不会重复投递。
>
> 边界提醒：**没有**任何远程 shell；cloud 只投递白名单 action 的 JSON，不执行命令、不拼接 shell。
> agent 上报的 `capabilities` 仅作记录，不作为可下发命令列表。

## 连接入口

```
GET /api/agent/ws
```

agent 以设备长期凭证（enroll 获得）发起 WebSocket 握手，鉴权 header：

```
X-Mgate-Device-ID: dev_xxx
Authorization: Bearer mgdt_xxx
```

鉴权在 **WebSocket 升级之前**完成，校验：

1. `X-Mgate-Device-ID` 与 `Authorization: Bearer <device_token>` 均存在。
2. 设备存在，且状态为 `enabled`（`pending`/`disabled`/`deleted` 一律拒绝）。
3. 存在 `active` 且未吊销的凭证，其 `token_hash` 与提交 token 的哈希**恒定时间比较**一致。

校验失败：返回 **401**（凭据缺失/无效/设备未就绪）或 **403**（设备被禁用），
**不升级** WebSocket，并写审计 `device.ws.auth_failed`（绝不记录 token）。

校验成功：升级连接，写审计 `device.ws.connect`；断开时写 `device.ws.disconnect`。

## 消息信封

所有消息使用统一 JSON 信封：

```json
{
  "v": 1,
  "id": "msg_xxx",
  "type": "agent.heartbeat",
  "ts": "2026-06-23T10:00:00Z",
  "device_id": "dev_xxx",
  "payload": {}
}
```

约束：

- `v` 必须为 `1`；`id` / `type` / `ts` 必填；`device_id` 必须等于已认证设备。
- 单条消息大小受限（默认 64 KiB，`MGATE_WS_MAX_MESSAGE_BYTES`），超限连接被关闭。
- JSON 严格解析；解析失败 / 字段缺失 → 回 `error` 信封（连接保留），**不 panic**。
- `device_id` 不匹配 → 回 `error` 并关闭连接（视为协议违规）。
- **未知 type → 回 `error` 信封，连接保留**（已选定行为，不关闭连接）。

## 支持的消息类型

agent → cloud：`agent.hello`、`agent.heartbeat`、`agent.status`，以及（Phase 4）`command.ack`、`command.result`
cloud → agent：`server.hello`、`server.pong`、`error`，以及（Phase 4）`command.deliver`

> 命令通道消息（`command.deliver/ack/result`）的字段与处理见 [commands.md](commands.md)。
> 严禁出现 `exec.raw` / `shell` / `bash` / `script` 等任何让 cloud 直接控制系统的消息。

### agent.hello

建连后首先发送，上报设备信息与能力声明：

```json
{
  "v": 1, "id": "msg_001", "type": "agent.hello",
  "ts": "2026-06-23T10:00:00Z", "device_id": "dev_xxx",
  "payload": {
    "agent_version": "0.1.0",
    "mgate_version": "0.3.7",
    "hostname": "mgate-ufi-001",
    "device_model": "ufi",
    "firmware_info": "debian",
    "capabilities": ["ap.status", "wlan.list", "doctor.full"]
  }
}
```

cloud：更新设备 `agent_version`/`mgate_version`/`hostname`/`device_model`/`firmware_info`/
`capabilities_json`/`last_seen_at`，写审计 `device.hello`，回 `server.hello`：

```json
{
  "v": 1, "id": "msg_server_001", "type": "server.hello",
  "ts": "2026-06-23T10:00:01Z", "device_id": "dev_xxx",
  "payload": { "server_time": "2026-06-23T10:00:01Z", "heartbeat_interval_sec": 25 }
}
```

### agent.heartbeat

按 `heartbeat_interval_sec` 周期发送：

```json
{
  "v": 1, "id": "msg_002", "type": "agent.heartbeat",
  "ts": "2026-06-23T10:00:25Z", "device_id": "dev_xxx",
  "payload": { "uptime_sec": 12345 }
}
```

cloud：刷新内存心跳时间与 DB `last_seen_at`，回 `server.pong`。
**心跳默认不写审计**，避免日志刷屏。

### agent.status

上报基础状态（结构由 agent 决定，cloud 原样保存）：

```json
{
  "v": 1, "id": "msg_003", "type": "agent.status",
  "ts": "2026-06-23T10:00:30Z", "device_id": "dev_xxx",
  "payload": {
    "ap": { "state": "running", "ssid": "Mgate-XXXX" },
    "wlan": { "state": "connected", "ssid": "HomeWiFi" },
    "tproxy": { "state": "enabled", "current_node": "US" },
    "gateway": { "state": "running" },
    "system": { "uptime_sec": 12345, "load": [0.2, 0.1, 0.1] }
  }
}
```

cloud：覆盖写入 `device_latest_status`，追加 `device_status_snapshots`，刷新 `last_seen_at`，
写审计 `device.status.reported`（**仅记录顶层字段名摘要，绝不保存完整大 JSON**）。

## 在线状态判定

```
online = Hub 中存在该设备的活跃连接，且最近心跳在 MGATE_WS_OFFLINE_AFTER_SEC 阈值内
```

要点：

- `online` 是**进程内连接的瞬时状态，不持久化**。Hub（进程）重启后所有设备自然离线，
  但数据库中的 `last_seen_at` / `last_ws_connected_at` 等时间点保留。
- 同一设备只保留**一个**活跃连接：新连接接入会**关闭并取代**旧连接。
- 读超时（= offline_after）与后台 reaper 共同保证失活连接被及时清理。
- 服务优雅关闭时，主动关闭所有连接。

## 部署注意（Cloudflare 443 / WSS）

- 经 Cloudflare 时使用 `wss://<host>/api/agent/ws`；Cloudflare 默认支持 WebSocket 透传。
- 反向代理需放行 `Upgrade` / `Connection` 头，并透传 `X-Forwarded-For`（审计取首段 IP）。
- 鉴权基于 bearer token（非 cookie），故 WebSocket 升级放开 Origin 校验是安全的。
- 注意代理的空闲超时应大于心跳间隔，避免连接被中间层提前断开。

## 调试：agent 模拟器

仓库提供仅用于开发的模拟器 `cmd/mgate-agent-sim`（不含任何设备控制能力）：

```bash
go run ./cmd/mgate-agent-sim \
  -gateway http://127.0.0.1:8080 \
  -device-id dev_xxx \
  -token mgdt_xxx \
  -heartbeat 25s -status
```

它会连接 WS、发送 `agent.hello`、周期 `agent.heartbeat`、可选一次 `agent.status`，
并打印 cloud 下发的 `server.hello` / `server.pong` / `error`。

## 配置项

| 变量 | 默认 | 说明 |
|------|------|------|
| `MGATE_WS_HEARTBEAT_INTERVAL_SEC` | `25` | 下发给 agent 的建议心跳间隔 |
| `MGATE_WS_OFFLINE_AFTER_SEC` | `90` | 离线判定阈值；读超时与清理也以此为界 |
| `MGATE_WS_MAX_MESSAGE_BYTES` | `65536` | 单条消息大小上限 |

## 不在本阶段（刻意排除）

- ❌ HTTPS Pull 与离线命令补偿
- ❌ 任意远程 shell（SSH / `bash -c` / `os/exec`）
- ❌ cloud 执行命令 / 拼接 mgate.sh（命令通道只下发白名单 action 的 JSON）
- ❌ 把 `capabilities` 当作可执行命令使用
