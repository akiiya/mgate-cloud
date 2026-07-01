# Agent Pull 兜底通道 📥

本文描述 HTTPS Pull 兜底通道：当 agent 无法保持 WebSocket 长连接时，
通过周期性 HTTPS 轮询完成心跳、状态上报、命令领取与 ack/result 提交。

> 边界提醒：Pull 与 WS 一样，cloud 只校验白名单 action 并下发 JSON，**不执行命令、不拼接 shell**；
> 真正调用设备本地 `mgate.sh` 是 `mgate-agent` 的职责。控制面无 Telegram、无远程 shell。

## 为什么需要 Pull

WebSocket 适合保持长连接、低延迟下发；但设备网络受限（NAT、代理、移动网络）时长连接可能不可用。
Pull 提供**兜底**：即便没有 WS，agent 也能定期“拉取”命令并回传结果。两者共用同一套
设备鉴权与命令 lease 机制，因此**同一条命令不会被 WS 与 Pull 重复投递**。

## 入口与鉴权

```
POST /api/agent/pull
X-Mgate-Device-ID: dev_xxx
Authorization: Bearer mgdt_xxx
Content-Type: application/json
```

鉴权与 WebSocket 完全一致：设备存在、`enabled`、有 `active` 凭证、token 哈希恒定时间比较。
`pending` / `disabled` / `deleted` 设备一律拒绝。鉴权失败写 `device.pull.auth_failed`（绝不记录 token）。
请求体上限 `MGATE_PULL_MAX_BODY_BYTES`（默认 128 KiB），严格 JSON 解析（拒绝未知字段）。

## 请求格式

```json
{
  "agent_version": "0.1.0",
  "mgate_version": "0.3.7",
  "hostname": "mgate-ufi-001",
  "device_model": "ufi",
  "firmware_info": "debian",
  "capabilities": ["ap.status", "wlan.list"],
  "status": { "ap": { "state": "running" } },
  "acks": [ { "command_id": "cmd_x", "accepted": true, "message": "accepted" } ],
  "results": [ { "command_id": "cmd_x", "status": "succeeded", "exit_code": 0, "stdout": "ok", "result": {} } ],
  "max_commands": 3
}
```

- `status` / `acks` / `results` / `max_commands` 均可选；`max_commands` 默认 1、上限 `MGATE_PULL_MAX_COMMANDS`（10）。
- `capabilities` 只记录，**不**用于绕过 allowlist。
- `acks` / `results` 仅能针对**本设备**的命令；跨设备命令会被拒绝（不改状态）。

## 响应格式

```json
{
  "ok": true,
  "data": {
    "server_time": "2026-06-23T10:00:00Z",
    "next_pull_after_sec": 15,
    "commands": [
      { "command_id": "cmd_x", "action": "ap.status", "params": {}, "timeout_sec": 60 }
    ]
  }
}
```

- `commands` 只含**本设备**、**白名单 action** 的命令，载荷绝无 shell/cmd/script/args/raw。
- 返回前命令已被 lease 并置为 `sent`；无命令时返回空数组（不报错）。

## 处理顺序

`POST /api/agent/pull` 内部顺序：

1. 鉴权（失败即返回，不处理业务）。
2. 解析请求（限大小、拒未知字段）。
3. 更新设备：`last_seen_at` / `last_pull_at` / 版本 / hostname / capabilities。
4. 若带 `status`：更新 `device_latest_status`（`source=pull`）、插入快照、更新 `last_pull_status_at`，写 `device.pull.status_reported`。
5. 处理 `acks`（复用命令 ack 逻辑，校验命令归属）。
6. 处理 `results`（复用命令 result 逻辑，截断、幂等；单条异常不影响整体）。
7. lease 待投递命令并置 `sent`，放入响应。
8. 若领取到命令，写 `device.pull`（metadata 仅记 count）。

## Pull 与 WS 如何避免重复投递

两通道共用 `commands` 表的 **lease 机制**：

- 领取命令即 `pending → leased`（带 `lease_until` 租约、`attempts+1`、`leased_by` 标记通道：`ws:<…>` 或 `pull:<request_id>`），随后置 `sent`。
- 领取是**原子条件更新**（配合 SQLite `_txlock=immediate`），同一命令只会被一个通道领取成功。
- 已 `sent`/终态的命令不会再被领取。
- WS 重连（hello 成功）会扫描该设备 pending 命令并尝试投递；Pull 则在请求时领取。二者互斥。

## 离线命令队列

**enabled 设备无论在线与否都可创建命令**。

- 在线且 WS 投递成功 → `delivered_via_ws`
- 在线但投递失败 → `queued_for_retry`（保持 pending，等待重试）
- 离线 → `device_offline_waiting_for_pull`（保持 pending，等待 Pull 或 WS 重连）

`pending`/`disabled`/`deleted` 设备仍禁止创建命令。

## 命令重试 / lease / reaper

后台 reaper（`MGATE_COMMAND_REAPER_INTERVAL_SEC`）：

- `leased` 租约过期、或 `sent`/`acked`/`running` 超过 `timeout_sec`：
  - `attempts < max_attempts` → 退回 `pending`（带退避 `MGATE_COMMAND_RETRY_BACKOFF_SEC`），写 `command.retry`；
  - `attempts >= max_attempts` → `timeout`。
- `pending` 超过 `expires_at`（`MGATE_COMMAND_PENDING_TTL_MINUTES`）→ `expired`。
- `canceled` / 终态命令不重试。

`max_attempts` 默认 `MGATE_COMMAND_DEFAULT_MAX_ATTEMPTS`（3）；`attempts` 在每次领取时 +1。

## last_pull_at 与 online 的区别

- `online`（WS 在线）：**进程内长连接**的瞬时状态；进程重启后归零。
- `last_pull_at`：设备最近一次通过 **HTTPS Pull** 联系的时间点；**持久化**。
- 后台连接状态据此区分：**WS 在线** / **最近 Pull** / **离线**。Pull 心跳**不**等于 WS 在线。

## 配置项

| 变量 | 默认 | 说明 |
|------|------|------|
| `MGATE_PULL_DEFAULT_INTERVAL_SEC` | `15` | 响应中建议 agent 的下次 Pull 间隔 |
| `MGATE_PULL_MAX_COMMANDS` | `10` | 单次 Pull 返回命令数上限 |
| `MGATE_PULL_MAX_BODY_BYTES` | `131072` | Pull 请求体大小上限 |
| `MGATE_COMMAND_DEFAULT_MAX_ATTEMPTS` | `3` | 命令默认最大投递尝试次数 |
| `MGATE_COMMAND_LEASE_SECONDS` | `60` | 投递租约时长 |
| `MGATE_COMMAND_RETRY_BACKOFF_SEC` | `10` | 重试退避 |

## 模拟器（开发联调）

```bash
go run ./cmd/mgate-agent-sim --mode pull \
  -gateway http://127.0.0.1:8080 -device-id dev_xxx -token mgdt_xxx -pull-interval 5s
```

它会周期 POST `/api/agent/pull`、上报状态、领取命令，并在下一轮提交 ack 与（模拟的）result。
模拟器**不执行任何真实命令**、不调用 mgate.sh、不引入 shell 能力，仅供开发。
