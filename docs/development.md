# 本地开发指南 🛠️

## 环境要求

- Go ≥ 1.26
- Node ≥ 18（含 npm）

## 目录速览

```
cmd/mgate-cloud      程序入口
internal/            后端各分层（app/admin/auth/audit/db/...）
migrations/          SQL 迁移（内嵌）
web/                 前端（React + Vite + Tailwind）
docs/                文档
scripts/             dev / build 脚本
```

## 分支与协作流程 🌿

> 重要：`main` 是受保护分支，请勿直接推送或改写其历史，避免误操作。

- **`main`：受保护分支**，禁止 `force-push`，禁止直接 `git push`。
- **日常开发在 `dev`**（或自 `dev` 切出的特性分支），所有改动先进 `dev`。
- 合入 `main` **只通过 GitHub 页面的 PR / 手动 merge**，不在本地直接推 `main`。
- `main` 合并后由 GitHub Actions **自动**：跑测试 → 按 `VERSION` 打 tag → 打包 → 发布 Release。
- **不要在本地手动打正式版本 tag**（如 `vX.Y.Z`），除非文档另有明确说明——版本 tag 交由 `main` 的 workflow 自动创建，以免与远程冲突或重复发布。
- 发布新版本：在 `dev` 更新 `VERSION` 与 `CHANGELOG.md`，经 PR 合并到 `main`，由 workflow 自动发版。

## 开发模式（推荐：前后端分离）

后端与前端各起一个进程，互不干扰、独立重启。

```bash
# 终端 A —— 后端
bash scripts/dev.sh
# 等价于：
#   MGATE_ADMIN_USERNAME=admin MGATE_ADMIN_PASSWORD=change-me go run ./cmd/mgate-cloud

# 终端 B —— 前端（热更新）
npm --prefix web install      # 首次
npm --prefix web run dev
```

- 前端开发地址：<http://127.0.0.1:5173/#/login>
- Vite 已配置把 `/api` 代理到后端 `:8080`，因此前端与 API 同源，cookie/CSRF 正常工作。

> Windows 下若不便运行 `.sh`，可直接执行其中的命令，或用 Git Bash。

## 仅用内嵌前端调试

若想验证"内嵌 SPA + 单二进制"的真实形态：

```bash
npm --prefix web run build           # 先构建一次前端
go run ./cmd/mgate-cloud             # 后端直接提供内嵌 SPA
# 访问 http://127.0.0.1:8080/#/login
```

## 测试

```bash
go test ./...                # 全部后端测试
go test ./... -run TestLogin -v
```

测试覆盖：配置加载、口令哈希/校验、令牌生成、迁移幂等、登录成功/失败、
`/api/auth/me` 未登录 401、healthz、CSRF 缺失拒绝、静态资源、审计写入与脱敏；
以及 Phase 2 的设备码编解码、enroll 各分支（成功/重复/过期/篡改/禁用）、
并发闸门、设备启停状态流转、设备写操作 CSRF 校验、审计不含明文。

前端类型检查随构建执行（`tsc --noEmit`）：

```bash
npm --prefix web run build
```

## 代码风格

- Go：`gofmt`/`go vet` 通过；handler / service / repository 边界清晰；
  错误统一返回；敏感字段不入普通日志。
- 前端：TypeScript 严格模式；组件小而清楚；请求逻辑集中在 `web/src/api/`；
  统一 UI 组件（`web/src/components/ui/`）与统一错误提示。
- 注释以中文为主，解释"为什么"，避免机械复述代码。

## 新增一张表 / 一次迁移

1. 在 `migrations/` 下新增 `NNN_xxx.sql`（序号递增，字典序即执行顺序）。
2. 迁移器启动时自动应用，幂等无需手动登记。
3. 如需新实体，在 `internal/model` 增结构体，并在对应 store 增 SQL 方法。

## 设备相关接口速览（Phase 2）

管理员接口（需登录；写操作需 CSRF）：

```text
GET  /api/admin/devices                          设备列表
POST /api/admin/devices                          创建设备
GET  /api/admin/devices/{device_id}              设备详情
POST /api/admin/devices/{device_id}/pairing-code 生成一次性设备码
POST /api/admin/devices/{device_id}/disable      禁用设备
POST /api/admin/devices/{device_id}/enable       启用设备
```

管理员命令接口（需登录；写操作需 CSRF）：

```text
GET  /api/admin/commands                          命令列表（device_id/status/limit 过滤）
GET  /api/admin/commands/{command_id}             命令详情 + 结果
POST /api/admin/devices/{device_id}/commands      创建命令（仅在线 enabled 设备）
POST /api/admin/commands/{command_id}/cancel      取消命令
```

设备 agent 公开接口（无 session、无 CSRF）：

```text
POST /api/agent/enroll                           使用设备码绑定
GET  /api/agent/ws                               WebSocket 接入（bearer token 鉴权）
POST /api/agent/pull                             HTTPS Pull 兜底（bearer token 鉴权）
```

enroll 的 curl 示例与设备码协议见 [protocol.md](protocol.md)；
WebSocket 鉴权、信封与消息类型见 [agent-ws.md](agent-ws.md)；
命令白名单、参数规则与状态机见 [commands.md](commands.md)；
HTTPS Pull 协议、离线队列与重试见 [agent-pull.md](agent-pull.md)。

## Agent 模拟器（开发联调）

仓库提供仅供开发的 WebSocket 模拟器（不含任何设备控制能力）：

```bash
# WS 模式（默认）
go run ./cmd/mgate-agent-sim --mode ws \
  -gateway http://127.0.0.1:8080 -device-id dev_xxx -token mgdt_xxx -heartbeat 25s -status

# Pull 模式（HTTPS 兜底）
go run ./cmd/mgate-agent-sim --mode pull \
  -gateway http://127.0.0.1:8080 -device-id dev_xxx -token mgdt_xxx -pull-interval 5s
```

它会连接 `/api/agent/ws`、发送 `agent.hello`、周期 `agent.heartbeat`、可选一次 `agent.status`，
并打印 cloud 下发的 `server.hello` / `server.pong` / `error`。
Phase 4 起它还会对收到的 `command.deliver` 返回 `command.ack` 与模拟的 `command.result`
（**不执行任何真实命令**）。该工具不打入最终 `mgate-cloud` 二进制。

## 手动验证 Phase 3 在线状态

1. 启动后端，登录后台。
2. 创建设备 → 生成设备码 → `curl` enroll，记录 `device_id` 与 `device_token`。
3. 运行模拟器连接 WebSocket。
4. `/#/devices` 应显示该设备 **在线**；详情页可见最新状态 JSON 与连接时间。
5. 停止模拟器，刷新后应变 **离线**，`最近断开` 时间更新。
6. 查 `audit_logs`：应有 `device.ws.connect` / `device.ws.disconnect` / `device.hello` / `device.status.reported`。

## 手动验证 Phase 4 命令通道

1. 完成上面的设备绑定与模拟器连接。
2. 设备详情页「远程操作」点击 `ap.status`（或 curl `POST /api/admin/devices/{id}/commands`）。
3. 命令应从 `sent` → `acked` → `succeeded`；命令详情页可见 stdout/stderr/result。
4. 停止模拟器后再创建命令，应返回 `device_offline`。
5. 尝试非白名单 action（如 `exec.raw`）应返回 `invalid_action`；夹带 `shell` 字段应返回 `invalid_params`。
6. 查 `audit_logs`：应有 `command.create` / `command.deliver` / `command.ack` / `command.result`。

## 手动验证 Phase 5 Pull 兜底

1. 完成绑定（得到 device_id + device_token）。
2. **离线创建**：不连 WS，在设备详情创建 `ap.status` → 命令应为 `pending`，hint `device_offline_waiting_for_pull`。
3. **Pull 领取**：`go run ./cmd/mgate-agent-sim --mode pull -device-id ... -token ...`；
   命令应被领取并在随后 `ack`/`result` 后变 `succeeded`。
4. 设备详情应显示「最近 Pull」、`last_pull_at` 更新、`latest_status` 来源为 `pull`，但 `online` 仍为离线。
5. **重试**：让模拟器领取后停止（不回 result），等待租约/超时；命令应 `command.retry` 回 pending，
   达 `max_attempts` 后 `timeout`。
6. 查 `audit_logs`：应有 `device.pull` / `command.retry` / `command.result` 等事件。

## 常见问题

- **`go build` 报找不到 `dist`**：执行 `npm --prefix web run build` 生成前端产物；
  仓库保留 `web/dist/.gitkeep` 保证最低限度可编译。
- **首页提示"前端资源未就绪"**：同上，需先构建前端。
- **登录 403 csrf_failed**：写请求需先 `GET /api/auth/csrf` 获取令牌并随 `X-CSRF-Token` 头提交；前端 API client 已自动处理。
