# 架构说明 🧱

本文描述 mgate-cloud 的整体架构、分层职责与安全边界。当前状态：**v0.1.0-rc2**
（管理后台 + 设备身份 + WS 连接 + 命令通道 + HTTPS Pull 兜底；并含无配置启动/Setup、检查更新/自更新与发布工程硬化）。

## 定位

mgate-cloud 是部署在公网的**设备管理控制面**：

- 面向管理员提供 Web 后台（经 Cloudflare 443 提供 HTTPS/WSS）。
- 将来作为 `mgate-agent` 主动连接的服务端。
- **它是控制面，不是执行器**：自身不触碰设备系统命令。

## 进程模型

单进程、单体服务、单可执行文件：

- 一个 Go 进程同时提供 `/api/*`（JSON 接口）与 `/`（内嵌 SPA）。
- 数据存储为进程内打开的嵌入式 SQLite（WAL 模式）。
- 不依赖 Redis / MQTT / Postgres / Kafka / NATS 等任何外部中间件。

```
cmd/mgate-cloud (main)
        │  加载 config，构造 app，启动 http.Server，处理优雅关闭
        ▼
internal/app  ── 组合根（composition root）
        │  装配 db / store / service / handler，编排路由与中间件
        ├── internal/admin   HTTP 适配层：认证 handler + 设备管理 handler + 命令 handler + 中间件
        ├── internal/agent   设备 agent 公开接口：enroll / WebSocket / HTTPS Pull（无 session / 无 CSRF）
        ├── internal/auth    业务层：登录、会话、CSRF、口令（含 store）
        ├── internal/device  设备身份与状态业务：设备/凭证/设备码/状态 store + 编解码 + service
        ├── internal/command 命令队列：白名单、store、dispatcher、result、reaper、service
        ├── internal/hub     Agent WebSocket 连接注册表与在线状态（纯内存，不触库）
        ├── internal/audit   审计：Record + request-id / 客户端 IP（可信代理策略）中间件
        ├── internal/version 构建版本（ldflags 注入）
        ├── internal/securitycheck 静态安全测试：禁止 os/exec 等远程 shell 能力
        ├── internal/api     稳定的响应信封与错误码
        ├── internal/db      连接、PRAGMA、迁移、事务
        ├── internal/model   领域实体（与表对应）
        ├── internal/webui   内嵌 SPA 的静态服务
        └── internal/util    id / clock / redact 等通用工具
```

## 分层与依赖方向

依赖自上而下、单向流动，便于测试与替换：

```
handler (admin)  →  service (auth)  →  store/repository  →  *sql.DB
   │                    │
   └── api(响应)        └── model(实体)
```

- **handler**：只做 HTTP 适配——解析请求、调用 service、写响应。不含业务逻辑。
- **service**：编排业务用例（如"登录 = 校验凭据 + 建会话 + 更新登录时间"）。
- **store/repository**：唯一的 SQL 落点，集中管理数据库访问。
- **model**：纯数据结构，只含与持久化无关的领域判断（如 `IsActive`）。

横切关注点（审计、请求 ID、响应契约、脱敏）单独成包，避免污染业务层。

## 请求流转（以登录为例）

```
POST /api/auth/login
  → RequestID 中间件（生成 request_id 注入 context）
  → recover 中间件（兜底 panic）
  → RequireCSRF（双提交校验 X-CSRF-Token == csrf cookie）
  → admin.Handlers.Login
       → auth.Service.Login（校验凭据 → 建会话 → 更新登录时间）
       → 写会话 Cookie（HttpOnly/SameSite/Secure）
       → audit.Record(admin.login.success)
       → api.WriteSuccess
```

## 数据模型

| 表 | 用途 | 关键点 |
|----|------|--------|
| `schema_migrations` | 迁移版本记录 | 由迁移器维护，保证幂等 |
| `admins` | 管理员账户 | 仅存 `password_hash`，明文永不落库 |
| `admin_sessions` | 服务端会话 | 仅存 `session_token_hash`，软删除式吊销（`revoked_at`） |
| `audit_logs` | 审计日志 | `metadata_json` 写库前脱敏 |
| `devices` | 设备 | 状态机 pending/enabled/disabled/deleted；enroll 时填充自述信息 |
| `device_credentials` | 设备长期凭证 | 仅存 `token_hash`，明文 `device_token` 只返回一次 |
| `device_pairing_codes` | 一次性设备码 | 仅存 `code_hash`；`used_at` 实现一次性 |

主键统一使用应用层生成的随机 ID（`crypto/rand`），不暴露创建顺序、无碰撞顾虑。

## 设备身份（Phase 2）

设备身份闭环由 `internal/device` 承载，分层与认证一致（handler → service → store → db）：

- `internal/admin/device_handlers.go`：管理员侧设备接口（需登录；写操作需 CSRF）。
- `internal/agent/handlers.go`：设备 agent 公开接口（仅 `enroll`，无 session、无 CSRF）。
- `internal/device/`：`service.go`（编排）、`store.go` / `credential.go` / `pairing.go`（持久化 + 设备码编解码）。

### enroll 流程（事务化、并发安全）

```
POST /api/agent/enroll  (无 session / 无 CSRF；信任来自"持有有效一次性设备码")
  → 离线校验设备码 HMAC 签名并解析载荷（不触库）
  → BEGIN TX
       查设备码(by code_hash) → 校验 未使用/未过期/设备未禁用/未绑定
       UPDATE used_at WHERE used_at IS NULL  ← 并发闸门：仅 1 行命中者继续
       生成 device_token（仅存哈希）写入 device_credentials
       设备置 enabled，写入自述信息与 last_enrolled_at
     COMMIT
  → 返回 device_id + device_token（仅此一次明文）
```

并发重复提交同一设备码时，"把 used_at 从 NULL 翻转成功"的事务唯一，其余收到失败，
因此**只会成功一次**。详见 [protocol.md](protocol.md)。

### 为何设备码不含永久密钥

设备码是泄露面较大的临时凭据，因此只携带**一次性、可过期**的 pairing token，
绝不包含长期 `device_token`；长期凭证只在 enroll 成功时返回一次并仅存哈希。
这样设备码即便泄露，影响也被限制在"有效期内、未被抢先使用前的一次尝试"。

## Agent 连接（Phase 3）

绑定后，agent 用 `device_id + device_token` 经 WebSocket 主动连接 cloud。

```
GET /api/agent/ws  (鉴权在升级前完成；失败返回 401/403，不升级)
  → device.AuthenticateDevice：设备 enabled + active 凭证 + token 哈希恒定时间比较
  → websocket.Accept（OriginPatterns:* —— 鉴权基于 token 而非 cookie，放开 Origin 安全）
  → hub.Register（同设备旧连接被取代并关闭）
  → 读循环：每条消息读超时 = offline_after
       agent.hello     → 更新设备信息/能力 + 回 server.hello
       agent.heartbeat → 刷新心跳 + last_seen_at + 回 server.pong
       agent.status    → 覆盖最新状态 + 追加快照 + 刷新 last_seen_at
  → 断开：hub.Unregister（connID 守卫）+ 记录 last_ws_disconnected_at + 审计
```

### Hub 与在线状态

- `internal/hub` 只维护 `map[device_id]*Connection` 与在线判断，**不触库**，因此可独立单测。
- `online = 存在活跃连接 且 最近心跳在 offline_after 阈值内`，是**进程内瞬时状态**，不持久化。
- 同设备单连接：`Register` 返回被取代的旧连接由调用方在锁外关闭；`Unregister` 带 `connID`
  守卫，避免被取代的旧连接在其读循环结束时误删接管者。
- 失活清理双保险：读循环按 `offline_after` 设读超时；后台 reaper 周期关闭超时连接。
- `device.Service` 通过 `Presence` 接口（由 hub 实现）查询在线状态，避免 `device→hub` 硬依赖，
  也使设备服务在无连接层时（presence=nil）可独立测试。

### 边界

WebSocket 通道承载 `hello/heartbeat/status` 与 `server.hello/pong/error`；
Phase 4 起额外承载命令通道 `command.deliver`（cloud→agent）与 `command.ack/result`（agent→cloud）。
`capabilities` 仅记录，不用于下发命令。

## 命令通道（Phase 4）

`internal/command` 负责命令的创建、投递、回收与清理，分层：service → store/result → db；
投递依赖 `Deliverer` 接口（由 `internal/agent` 基于 Hub 实现，避免 command→agent 导入环）。

```
管理员 POST /api/admin/devices/{id}/commands
  → device.EnsureCommandable（必须 enabled + 在线）
  → allowlist 校验 action + 严格校验/规范化 params（拒绝未知/危险字段）
  → store.Insert（落库 pending）         ← 先落库
  → audit command.create
  → dispatcher.TryDeliver：lease(pending→leased) → Deliverer.Deliver(WS) → MarkSent(→sent)
       投递失败 → ReleaseLease(→pending)，命令不丢
  → audit command.deliver（投递成功时）

agent command.ack    → MarkAcked / 或 failed+result（rejected）
agent command.result → 截断 → results.InsertIfAbsent（幂等）→ ApplyResultStatus（终态，不覆盖已终态）
reaper（后台）       → 进行中超 timeout_sec → timeout；pending 超 expires_at → expired
```

### 不变量与安全

- **先落库再投递**：命令必存在于 `commands` 表后才投递；投递失败保持 `pending`，绝不丢失。
- **状态机集中**：所有流转用带 `WHERE` 守卫的条件更新（store 层），handler 不直接改状态。
- **结果幂等**：`command_results.command_id` 唯一 + `ON CONFLICT DO NOTHING`，重发不产生多条、不翻回终态。
- **只投递白名单**：`command.deliver` 载荷仅 `command_id/action/params/timeout_sec`，绝无 shell/cmd/script/args/raw。
- **cloud 不执行**：无 `os/exec`/`exec.Command`/`bash`/`sh`；调用 `mgate.sh` 是 agent 的职责。
- **结果限长**：stdout/stderr/result 截断至上限并置 `truncated`；审计只记摘要，不记大输出与 token。

详见 [commands.md](commands.md)。

## HTTPS Pull 兜底（Phase 5）

当 agent 无法保持 WebSocket 时，经 `POST /api/agent/pull` 周期轮询完成心跳/状态/命令收发。

```
POST /api/agent/pull  (与 WS 相同的 device_id + device_token 鉴权)
  鉴权 → 解析(限大小/拒未知字段)
       → 更新 last_pull_at / 自述信息
       → status（source=pull，更新 last_pull_status_at）
       → 处理 acks / results（复用 Phase 4 逻辑，校验归属、截断、幂等）
       → LeaseForPull：领取 pending → 置 sent → 放入响应 commands
  响应 { server_time, next_pull_after_sec, commands[] }
```

### WS 与 Pull 的投递协调（共用 lease）

两通道共用 `commands` 表的 lease 机制，保证同一命令不被重复投递：

- 领取 = 原子条件更新 `pending → leased`（`attempts+1`、`lease_until` 租约、`leased_by` 记通道
  `ws:<instance>` / `pull:<request_id>`），随后置 `sent`。配合 `_txlock=immediate`，并发领取互斥。
- `pending` 的可领取条件含 `lease_until IS NULL OR lease_until <= now`，重试退避即复用 `lease_until`。
- WS 重连（hello 成功）扫描该设备 pending 命令并 `TryDeliver`；Pull 请求时 `LeaseForPull`。互斥不重复。

### 重试 / reaper（Phase 5 增强）

- `leased` 租约过期、或 `sent/acked/running` 超 `timeout_sec`：`attempts < max_attempts` → 退回
  `pending`（带退避，`command.retry`）；否则 → `timeout`。
- `pending` 超 `expires_at` → `expired`。`canceled`/终态不重试。

### 在线 vs 最近 Pull

`online` 是 Hub 进程内 WS 连接的瞬时状态；`last_pull_at` 是最近一次 HTTPS Pull 的持久化时间点。
二者不同：Pull 心跳不代表 WS 在线。前端据此区分「WS 在线 / 最近 Pull / 离线」。

详见 [agent-pull.md](agent-pull.md)。

## 路由与静态资源

- `/api/...` 由专用 mux 精确匹配（含 HTTP 方法）。
- `/` 挂载内嵌 SPA；因 `/api/` 前缀更具体，API 不会被 SPA 回退吞掉。
- SPA 找不到对应文件时回退 `index.html`，配合前端 **hash 路由**保证刷新/深链不 404。
- 前端构建产物经 `//go:embed all:dist` 内嵌，运行不依赖外部静态目录。

## 安全边界（必须长期保持）

mgate-cloud **刻意不具备**以下能力，且不应在任何阶段引入：

- 不 SSH 到设备；不执行 shell；不提供 `bash -c` / `sh -c`。
- 不提供 raw exec / 任意命令执行；代码层不引入 `os/exec` 等执行能力。

未来的设备控制路径（Phase 2+）将是：

```
管理员 → cloud（记录意图/下发白名单 action）
            ▲
            │ agent 主动连接（WSS），cloud 不主动连设备
        mgate-agent → 校验 action 在白名单内 → 调用设备本地 mgate.sh
```

即：**cloud 永远不直接触碰设备系统**，只通过受约束的 action 间接表达意图。

## 认证与会话安全

- 口令：bcrypt（cost=12），自带随机盐。
- 令牌：会话 / CSRF 令牌均由 `crypto/rand` 生成（256-bit），**入库前 SHA-256 哈希**。
- 会话 Cookie：`HttpOnly` + `SameSite=Lax` + 可选 `Secure`。
- CSRF：双提交模式（cookie + `X-CSRF-Token` 头，恒定时间比较）。
- 防账户枚举：登录失败统一返回 `invalid_credentials`。
- 脱敏：日志与审计绝不含口令/令牌/cookie；审计 metadata 经 `util.Redact` 过滤。

## 运维与发布硬化（v0.1.0-rc1）

- **存活/就绪**：`GET /api/healthz`（进程存活）、`GET /api/readyz`（数据库 Ping 通过才就绪，否则 503）。
- **版本**：`internal/version.Version` 由 ldflags 注入，启动日志输出"版本/模式/地址/DB 路径"，绝不输出 secret。
- **运行模式**：`MGATE_MODE`（dev/test/prod）。prod 下 `MGATE_APP_SECRET` 为空即拒绝启动。
- **真实客户端 IP**：自适配解析 `CF-Connecting-IP` → `X-Forwarded-For`(最左) → `RemoteAddr`，无需配置；供审计与登录限流使用。
- **静态安全闸门**：`internal/securitycheck` 测试扫描源码，禁止 `os/exec`/`exec.Command`/`bash -c`/`sh -c`。
