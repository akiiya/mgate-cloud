# 安全说明 🔐

本文汇总 `mgate-cloud` 的安全边界与硬化措施。安全边界是产品的硬约束，任何阶段都不得突破。

## 不可逾越的边界

mgate-cloud 是**公网控制面**，不是设备本地执行器。它**永远不**：

- ❌ SSH 登录设备；不执行 shell；不提供 `bash -c` / `sh -c` / raw exec。
- ❌ 拼接或调用设备本地 `mgate.sh` 命令行——真正执行是 `mgate-agent` 的职责。
- ❌ 让管理员输入任意命令字符串；命令只能是**白名单 action + 受限参数**。
- ❌ 把 agent 上报的 `capabilities` 当作可执行命令列表（仅记录）。

**静态保证**：`internal/securitycheck` 的测试扫描全部源码，一旦出现 `os/exec` /
`exec.Command` / `bash -c` / `sh -c` 即 CI 失败。

## 命令通道安全

- action 必须来自集中定义的白名单（`internal/command/allowlist.go`）。
- 参数严格校验：拒绝未知字段、拒绝危险字段（`shell`/`cmd`/`script`/`args`/`argv`/`extra_args`/`raw`/…）、限长、禁控制字符。
- WS 与 Pull 投递的载荷只含 `command_id/action/params/timeout_sec`，绝无可执行内容。
- ack/result 必须属于当前连接认证的 `device_id`，跨设备一律拒绝。
- 命令结果 `stdout/stderr/result` 落库有大小上限并截断；审计只记摘要，不记大输出。

## 认证与凭据

- 管理员口令：bcrypt（cost 12）。会话/CSRF/设备/pairing 令牌均 `crypto/rand` 生成，**入库前哈希**。
- 设备码：HMAC-SHA256 签名、可过期、一次性，不含永久密钥。
- 令牌比较使用**恒定时间比较**。
- 设备 WS/Pull 鉴权一致：`enabled` + `active` 凭证 + token 哈希恒定时间比较；`pending`/`disabled`/`deleted` 拒绝。

## Web 会话

- 会话 Cookie：`HttpOnly` + `SameSite=Lax`，是否 `Secure` 由 `MGATE_COOKIE_SECURE` 控制（HTTPS 部署必开）。
- 写请求需 CSRF（双提交：`X-CSRF-Token` 头 + cookie 比对，恒定时间）。
- 登录失败不区分"用户名不存在/口令错误"，防账户枚举。

## 生产硬化（v0.1.0-rc1）

- **APP_SECRET 强制**：`MGATE_MODE=prod` 且 `MGATE_APP_SECRET` 为空 → **拒绝启动**；dev/test 才临时生成。
- **可信代理策略**：仅当 `MGATE_TRUST_PROXY_HEADERS=true` 才采纳 `CF-Connecting-IP` / `X-Forwarded-For`
  （优先 CF-Connecting-IP）。默认 false，仅用 `RemoteAddr`，避免客户端伪造来源 IP。
- **启动日志**：输出版本 / 模式 / 监听地址 / DB 路径；配置打印对所有 secret 脱敏（显示 `(set)`/`(unset)`）。
- 请求体大小限制：enroll 16 KiB、WS 单帧 64 KiB、Pull 128 KiB；均严格 JSON、拒未知字段。

## 日志与审计脱敏

- 普通日志与审计 `metadata` **绝不**包含：口令、session/CSRF token、设备码、pairing/device token、cookie。
- 审计写入前经 `util.Redact` 过滤敏感字段；命令输出只写摘要。

## 部署建议

- 置于 TLS 终结之后（Cloudflare 443 / Caddy / Nginx），回源同源路径以保证 cookie/CSRF 正常。
- WebSocket（`/api/agent/ws`）需反代放行 `Upgrade`；空闲超时大于心跳间隔。
- 数据库目录最小可写权限；systemd 单元已加 `NoNewPrivileges` / `ProtectSystem=strict` 等加固。

详见 [deployment.md](deployment.md) 与 [release-checklist.md](release-checklist.md)。
