# 命令通道 🎛️

本文描述 Phase 4 的白名单命令队列：动作白名单、参数规则、消息协议、命令状态机、
结果存储与截断策略。

> 核心边界：cloud 只做 **校验白名单 action → 落库 command → 经 WS 投递 JSON → 保存结果**。
> cloud **不执行命令、不拼接 shell、不下发 raw command**；真正调用设备本地 `mgate.sh`
> 是 `mgate-agent` 的职责。代码中不含 `os/exec` / `exec.Command` / `bash -c` / `sh -c`。
>
> Phase 5 起：**enabled 设备无论在线与否都可创建命令**，离线命令进入队列等待 HTTPS Pull
> 或 WS 重连投递；WS 与 Pull 共用 lease 机制，互不重复投递。Pull 协议见 [agent-pull.md](agent-pull.md)。

## 白名单 Action

命令的 action 必须来自固定白名单（集中定义于 `internal/command/allowlist.go`）：

| Action | 参数 | 说明 |
|--------|------|------|
| `ap.start` / `ap.stop` / `ap.status` | `{}` | AP 控制/状态 |
| `wlan.scan` / `wlan.list` | `{}` | WLAN 扫描/列表 |
| `wlan.connect` | `{ssid}` 或 `{profile_id}` | 连接 WLAN（二选一，**无 password**） |
| `tproxy.enable` / `tproxy.disable` | `{}` | 透明代理开关 |
| `tproxy.use` | `{node}` | 切换节点 |
| `gateway.start` / `gateway.stop` / `gateway.status` | `{}` | 网关控制/状态 |
| `doctor.full` | `{}` | 全量自检 |

### 参数校验规则

- 非白名单 action 一律拒绝（`invalid_action`）。
- 无参数 action 只接受 `{}`，**任何字段**都会被拒绝。
- `wlan.connect`：`ssid` 与 `profile_id` 至少一个；字符串限长、禁控制字符；**禁止 password**。
- `tproxy.use`：`node` 必填、限长、禁控制字符。
- **危险字段纵深防御**：任何参数若含 `shell` / `cmd` / `command` / `script` / `args` /
  `argv` / `extra_args` / `raw` / `exec` / `bash` / `sh` / `password` 等键，一律拒绝（`invalid_params`）。
- 参数必须是 JSON **对象**；未知字段一律拒绝（`DisallowUnknownFields`）。

## 命令状态机

```
                      ┌───────────── cancel（管理员）──────────────┐
                      │                                            ▼
 pending ─lease→ leased ─deliver→ sent ─ack(accepted)→ acked ─result→ succeeded
   │                │                │                    │             /failed
   │                │(deliver 失败)   │                    │            /timeout
   │ expires_at 到   └──release→ pending                   │
   ▼                                                       │
 expired                          ack(rejected) → failed   │
                                  reaper 超时    → timeout ─┘
```

终态：`succeeded` / `failed` / `timeout` / `canceled` / `expired`（不再变更）。

要点：

- **先落库再投递**：命令一定先写入 `commands` 表，才会经 WS 投递；投递失败释放租约、保持
  `pending`，**绝不丢命令**。
- 状态流转通过带 `WHERE` 守卫的条件更新实现（集中在 `internal/command/store.go`），
  handler 不直接改状态。
- `command.result` **幂等**：`command_results.command_id` 唯一 + `ON CONFLICT DO NOTHING`，
  agent 重发结果不会产生多条、不会把已终态命令翻回。

## WebSocket 协议

在 Phase 3 的 `agent.hello/heartbeat/status` 与 `server.hello/pong/error` 之上新增：

cloud → agent：`command.deliver`
agent → cloud：`command.ack`、`command.result`

> 严禁 `exec.raw` / `shell` / `bash` / `script` 等任何控制类消息。

### command.deliver（cloud → agent）

```json
{
  "v": 1, "id": "msg_server_100", "type": "command.deliver",
  "ts": "2026-06-23T10:01:00Z", "device_id": "dev_xxx",
  "payload": { "command_id": "cmd_xxx", "action": "ap.status", "params": {}, "timeout_sec": 60 }
}
```

载荷**只含** `command_id` / `action` / `params` / `timeout_sec`——绝无 shell/cmd/script/args/raw 等字段。

### command.ack（agent → cloud）

```json
{ "type": "command.ack", "device_id": "dev_xxx",
  "payload": { "command_id": "cmd_xxx", "accepted": true, "message": "accepted" } }
```

- `accepted=true`：命令 `sent`/`leased` → `acked`。
- `accepted=false`：命令 → `failed`，并保存一条结果记录（`error_message` 为拒绝原因）。
- cloud 校验命令归属于当前连接的 `device_id`，否则忽略。

### command.result（agent → cloud）

```json
{ "type": "command.result", "device_id": "dev_xxx",
  "payload": {
    "command_id": "cmd_xxx", "status": "succeeded", "exit_code": 0,
    "stdout": "ok", "stderr": "", "result": { "state": "running" },
    "started_at": "...", "finished_at": "..."
  } }
}
```

合法结果状态：`succeeded` / `failed` / `timeout`（非法值按 `failed` 处理）。

cloud 处理：截断超长字段 → 保存 `command_results`（幂等）→ 更新命令终态 → 写审计。

## 结果存储与截断

- `stdout` / `stderr` / `result_json` 各自上限 `MGATE_COMMAND_RESULT_MAX_BYTES`（默认 256 KiB）。
- 超出即截断（修正到 UTF-8 边界），并置 `truncated=1`。
- 审计 metadata **只记录摘要**（action / status / exit_code / truncated），**绝不**写 stdout/stderr 全量，也不写任何 token。

## 超时 / 重试 / 过期（Phase 5 增强）

后台 reaper（`MGATE_COMMAND_REAPER_INTERVAL_SEC`，默认 10s）周期扫描：

- `leased` 租约过期、或 `sent`/`acked`/`running` 超过 `timeout_sec`：
  - `attempts < max_attempts` → 退回 `pending`（带退避 `MGATE_COMMAND_RETRY_BACKOFF_SEC`），审计 `command.retry`；
  - `attempts >= max_attempts` → `timeout`（审计 `command.timeout`）。
- `pending` 超过 `expires_at`（`MGATE_COMMAND_PENDING_TTL_MINUTES`，默认 10 分钟）→ `expired`（审计 `command.expired`）。
- `canceled` / 终态命令不重试。`max_attempts` 默认 `MGATE_COMMAND_DEFAULT_MAX_ATTEMPTS`（3），`attempts` 在每次领取时 +1。

服务优雅关闭时停止 reaper。

## 取消

`POST /api/admin/commands/{id}/cancel`：

- 仅 `pending`/`leased`/`sent`/`acked`/`running` 可取消（终态返回 `command_not_cancelable`）。
- Phase 4 的取消是 **cloud 侧标记**，**不**通知 agent，**不保证**终止 agent 已开始的本地执行。

## 管理员 API

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/api/admin/commands?device_id=&status=&limit=` | 命令列表 |
| GET | `/api/admin/commands/{command_id}` | 命令详情 + 结果 |
| POST | `/api/admin/devices/{device_id}/commands` | 创建命令（需 CSRF；enabled 设备，在线/离线均可） |
| POST | `/api/admin/commands/{command_id}/cancel` | 取消命令（需 CSRF） |

创建成功返回 `command` 与 `delivery_hint`（`delivered_via_ws` / `queued_for_retry` /
`device_offline_waiting_for_pull`）。创建失败错误码：`device_not_ready`（pending/disabled）/
`device_not_found` / `invalid_action` / `invalid_params` / `timeout_too_large`。

## 审计事件

`command.create`、`command.deliver`、`command.ack`、`command.rejected`、`command.result`、
`command.timeout`、`command.expired`、`command.cancel`。

## 配置项

| 变量 | 默认 | 说明 |
|------|------|------|
| `MGATE_COMMAND_DEFAULT_TIMEOUT_SEC` | `60` | 命令默认超时 |
| `MGATE_COMMAND_MAX_TIMEOUT_SEC` | `300` | 命令最大超时（创建上限） |
| `MGATE_COMMAND_RESULT_MAX_BYTES` | `262144` | stdout/stderr/result 各自字节上限 |
| `MGATE_COMMAND_PENDING_TTL_MINUTES` | `10` | pending 命令存活上限 |
| `MGATE_COMMAND_REAPER_INTERVAL_SEC` | `10` | 超时/过期扫描间隔 |
