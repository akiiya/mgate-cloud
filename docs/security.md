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
- **计时对齐**：用户名不存在 / 账户被禁用时也执行一次等价耗时的 bcrypt 比较（`auth.DummyVerify`），
  消除可据响应耗时枚举有效用户名的计时侧信道。

## 登录失败限流（在线暴力破解防护）

公网部署下管理员登录是重点攻击面，故对登录失败按**来源 IP** 限流并**升级封禁**：

- 同一 IP 在窗口（默认 15 分钟）内连续失败达到阈值（默认 5 次）即封禁；封禁期间登录直接返回
  `429 too_many_attempts` 并带 `Retry-After`，**在校验口令（含 bcrypt）之前**即拦截。
- **失败越多、封得越久**：封禁时长从基准（默认 1 小时）按封禁等级翻倍升级（1h→2h→4h…），上限默认 24 小时。
- 状态**持久化**于 `login_attempts` 表，重启不清空（攻击者无法通过触发重启绕过）。
- 登录成功清零该 IP 记录；距上次失败超过衰减期（默认 24 小时）后封禁等级归零，避免共享/动态 IP 被永久升级。
- 封禁按**真实客户端 IP**进行（IP 解析见上「真实客户端 IP」）；**绝不封禁本地回环/私有/链路本地等
  非公网地址**，避免把反代或基础设施 IP 拉黑而锁死所有人。Cloudflare/反代自身的 IP 已被还原为真实
  客户端，不会成为封禁对象。
- **管理员自我解封**（不慎把自己锁了）：①等封禁到期；②临时 `MGATE_LOGIN_THROTTLE_ENABLED=false`
  重启；③直接清库：`DELETE FROM login_attempts WHERE ip='<你的IP>';`。
- 相关配置（均有安全默认值）：`MGATE_LOGIN_THROTTLE_ENABLED`、`MGATE_LOGIN_MAX_FAILURES`、
  `MGATE_LOGIN_FAILURE_WINDOW_MINUTES`、`MGATE_LOGIN_BAN_BASE_MINUTES`、`MGATE_LOGIN_BAN_MAX_HOURS`、
  `MGATE_LOGIN_BAN_RESET_HOURS`。

## 生产硬化（v0.1.0-rc1）

- **APP_SECRET 强制**：`MGATE_MODE=prod` 且 `MGATE_APP_SECRET` 为空 → **拒绝启动**；dev/test 才临时生成。
- **真实客户端 IP（自适配，无需配置）**：按 `CF-Connecting-IP`（Cloudflare）→ `X-Forwarded-For`
  最左侧（其它反代）→ `RemoteAddr`（直连）顺序解析，自动适配各部署环境，拿到“登录者的真实来源 IP”
  供审计与限流使用。
  - **部署约定**：本服务应只经反代访问、不直接裸奔公网（绑定 `127.0.0.1`/私有网或防火墙限制）；
    此前提下转发头由可信反代设置，不会被外部伪造。
- **启动日志**：输出版本 / 模式 / 监听地址 / DB 路径；配置打印对所有 secret 脱敏（显示 `(set)`/`(unset)`）。
- 请求体大小限制：enroll 16 KiB、WS 单帧 64 KiB、Pull 128 KiB；均严格 JSON、拒未知字段。

## 初始化（Setup）安全

- Setup 只把管理员**口令哈希**（bcrypt）写入 `config.yaml`，**绝不**写明文密码。
- `app_secret` 留空时自动生成强随机；**生产模式必须有固定 app_secret**（否则启动被拒）。
- 生成的 `config.yaml` 权限收紧到 `0600`（Windows 权限语义有限，需自行确保访问控制）。
- Setup 完成前，除 setup/健康检查/静态页外的 API 一律返回 `setup_required`；完成后不可重复初始化。
- Setup 请求严格 JSON、限大小、字段校验；审计 `system.setup.completed` 不含密码/secret。

## 自更新安全边界

- 自更新**只**下载 GitHub Release 压缩包、**校验 SHA256**、解压并替换 **mgate-cloud 二进制本身**。
- **不使用** `os/exec` / `exec.Command` / `bash` / `sh`，**不执行任何外部命令或下载包内脚本**。
- 校验失败立即中止；替换前备份为 `.bak`，可回滚。
- Windows 无法替换运行中 exe 时，仅下载到 `.new` 并提示手动替换。
- 更新功能可由 `update_check_enabled=false` 关闭。详见 [update.md](update.md)。

## 日志与审计脱敏

- 普通日志与审计 `metadata` **绝不**包含：口令、session/CSRF token、设备码、pairing/device token、cookie。
- 审计写入前经 `util.Redact` 过滤敏感字段；命令输出只写摘要。

## 部署建议

- 置于 TLS 终结之后（Cloudflare 443 / Caddy / Nginx），回源同源路径以保证 cookie/CSRF 正常。
- WebSocket（`/api/agent/ws`）需反代放行 `Upgrade`；空闲超时大于心跳间隔。
- 数据库目录最小可写权限；systemd 单元已加 `NoNewPrivileges` / `ProtectSystem=strict` 等加固。

详见 [deployment.md](deployment.md) 与 [release-checklist.md](release-checklist.md)。
