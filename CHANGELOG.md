# 更新日志

本项目遵循语义化版本（SemVer）。日期为 UTC。

## v0.1.0-rc1 — 发布候选 1

首个发布候选版本：完成 Phase 1–5 的设备管理控制面，并经过发布前硬化。

### ✨ 功能

- **管理后台**：管理员登录/登出、Session Cookie、CSRF 双提交防护、审计日志、现代化 SPA（hash 路由）。
- **设备身份**：设备管理、一次性设备码（HMAC 签名、可过期、用后失效）、agent enroll、长期凭证（仅存哈希）。
- **Agent 连接**：WebSocket Hub、在线状态、心跳、基础状态上报；同设备单连接、超时自动清理。
- **命令通道**：白名单 action + 严格参数校验、先落库再投递、`command.ack/result` 回放、超时/过期清理。
- **Pull 兜底**：HTTPS Pull 离线轮询、离线命令队列、命令重试/退避；WS 与 Pull 共用 lease，不重复投递。

### 🔒 安全硬化（本次发布）

- 生产模式（`MGATE_MODE=prod`）下 `MGATE_APP_SECRET` 为空将**拒绝启动**；dev/test 可临时生成。
- 明确可信代理策略：仅当 `MGATE_TRUST_PROXY_HEADERS=true` 才采纳 `CF-Connecting-IP` / `X-Forwarded-For`。
- 新增静态安全测试：源码中出现 `os/exec` / `exec.Command` / `bash -c` / `sh -c` 即 CI 失败。
- 启动日志输出版本/模式/地址/DB 路径，**绝不**输出任何 secret。

### 🩺 运维

- `GET /api/healthz`：进程存活探测。
- `GET /api/readyz`：数据库可用才返回就绪（503 表示未就绪）。

### 📦 工程

- `VERSION` 文件 + 通过 ldflags 注入二进制版本。
- `Makefile`、`scripts/release.sh`、GitHub Actions CI 与 Release workflow。
- 部署资料：systemd、Caddy/Nginx 反代、Cloudflare WSS、数据库备份/恢复。

### 🚫 刻意不包含（安全边界）

- 不执行任何命令、不拼接 shell、不调用 mgate.sh（真正执行是 mgate-agent 的职责）。
- 无 SSH / raw exec / 远程 shell；无 Telegram Bot；无 Redis/MQTT/Postgres；不做多实例/集群。

### 已知限制

- 单进程在线/lease（未做多实例共享 presence）。
- 命令取消为 cloud 侧标记，不通知 agent。
- Pull 为固定间隔轮询（非长轮询）。
