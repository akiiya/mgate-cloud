# 发布清单 ✅

发布 `mgate-cloud` 前逐项确认。命令均可在本地执行，无需真正发版。

## 1. 代码与测试

- [ ] `go vet ./...` 通过
- [ ] `go test ./...` 通过（含 `internal/securitycheck` 静态安全测试）
- [ ] `gofmt -l` 无输出（格式整洁）
- [ ] `npm --prefix web run build` 通过（含 `tsc` 类型检查）
- [ ] `go build -o mgate-cloud ./cmd/mgate-cloud` 通过
- [ ] `bash scripts/build.sh` 生成 `dist/mgate-cloud`
- [ ] `bash scripts/release.sh` 生成多平台压缩包（tar.gz/zip）与 `SHA256SUMS`

> 脚本统一通过 `bash scripts/xxx.sh` 调用，**不依赖文件可执行位**（git 中为 `100644`，CI 直接执行会 `Permission denied` / exit 126）。
> 本地若想直接 `./scripts/xxx.sh`，可先 `chmod +x scripts/*.sh`；CI 与 Makefile 已统一用 `bash`，无需该执行位。

## 2. 安全边界

- [ ] 源码无 `os/exec` / `exec.Command` / `bash -c` / `sh -c`（由静态测试保证）
- [ ] 未引入 Telegram Bot / 真实 mgate.sh 调用 / 远程 shell
- [ ] 未引入 Redis / MQTT / Postgres；未做多实例/集群
- [ ] 日志与审计无 secret / token / cookie / 大输出（抽查 `audit_logs`）

## 3. 版本与产物

- [ ] 二进制启动日志显示正确版本（`mgate-cloud <version> 启动...`；版本来自 git tag / `git describe`）
- [ ] 最终二进制内嵌前端：删除源码 `web/dist` 后已编译二进制仍能提供页面

> 版本号机制（无 `VERSION` 文件、tag 为唯一来源）见 [releasing.md](releasing.md)；资产格式见 [release-assets.md](release-assets.md)。

## 3.5 首次初始化（Setup）

- [ ] 无 config.yaml、无环境变量时启动进入 Setup 模式，`/#/setup` 可访问
- [ ] Setup 完成生成 `config.yaml`，且**不含明文密码**（仅 `admin_password_hash`）
- [ ] 再次启动读取 config.yaml，不再进入 Setup

## 4. 生产配置核对（部署前）

- [ ] `MGATE_MODE=prod`
- [ ] `MGATE_APP_SECRET` 已设为固定高强度随机串（为空将拒绝启动）
- [ ] `MGATE_COOKIE_SECURE=true`（HTTPS 部署）
- [ ] `MGATE_BASE_URL` 为对外地址（影响下发给 agent 的 gateway/ws/pull）
- [ ] 仅经反代访问、本服务**不直接裸奔公网**（绑定 `127.0.0.1`/私有网或防火墙限制），保证真实 IP 解析可信
- [ ] 首次创建管理员后从环境移除 `MGATE_ADMIN_PASSWORD`
- [ ] 数据库目录可写、已纳入备份

## 5. 冒烟验证（启动后）

- [ ] `GET /api/healthz` 返回 ok
- [ ] `GET /api/readyz` 返回 ready
- [ ] 浏览器可登录后台
- [ ] agent 可经 WS 连接、可经 Pull 兜底
- [ ] 命令可下发并回收结果

## 6. 发布

- [ ] 所有改动在 `dev` 通过 CI，并经 PR 合并到 `main`
- [ ] 按 [releasing.md](releasing.md) 打 `vX.Y.Z` tag 触发发布（rc/beta 记得标 pre-release）
- [ ] 校验 Release 产物 `SHA256SUMS`

> 完整发版步骤（打 tag、Release Notes、重跑等）见 [releasing.md](releasing.md)。
