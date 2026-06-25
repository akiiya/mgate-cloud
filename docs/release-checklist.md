# 发布清单 ✅

发布 `mgate-cloud` 前逐项确认。命令均可在本地执行，无需真正发版。

## 1. 代码与测试

- [ ] `go vet ./...` 通过
- [ ] `go test ./...` 通过（含 `internal/securitycheck` 静态安全测试）
- [ ] `gofmt -l` 无输出（格式整洁）
- [ ] `npm --prefix web run build` 通过（含 `tsc` 类型检查）
- [ ] `go build -o mgate-cloud ./cmd/mgate-cloud` 通过
- [ ] `./scripts/build.sh` 生成 `dist/mgate-cloud`

## 2. 安全边界

- [ ] 源码无 `os/exec` / `exec.Command` / `bash -c` / `sh -c`（由静态测试保证）
- [ ] 未引入 Telegram Bot / 真实 mgate.sh 调用 / 远程 shell
- [ ] 未引入 Redis / MQTT / Postgres；未做多实例/集群
- [ ] 日志与审计无 secret / token / cookie / 大输出（抽查 `audit_logs`）

## 3. 版本与产物

- [ ] `VERSION` 文件已更新为目标版本（如 `0.1.0-rc1`）
- [ ] `CHANGELOG.md` 已补充该版本条目
- [ ] 二进制启动日志显示正确版本（`mgate-cloud <version> 启动...`）
- [ ] 最终二进制内嵌前端：删除源码 `web/dist` 后已编译二进制仍能提供页面

## 4. 生产配置核对（部署前）

- [ ] `MGATE_MODE=prod`
- [ ] `MGATE_APP_SECRET` 已设为固定高强度随机串（为空将拒绝启动）
- [ ] `MGATE_COOKIE_SECURE=true`（HTTPS 部署）
- [ ] `MGATE_BASE_URL` 为对外地址（影响下发给 agent 的 gateway/ws/pull）
- [ ] 置于可信反代之后时 `MGATE_TRUST_PROXY_HEADERS=true`，否则保持 false
- [ ] 首次创建管理员后从环境移除 `MGATE_ADMIN_PASSWORD`
- [ ] 数据库目录可写、已纳入备份

## 5. 冒烟验证（启动后）

- [ ] `GET /api/healthz` 返回 ok
- [ ] `GET /api/readyz` 返回 ready
- [ ] 浏览器可登录后台
- [ ] agent 可经 WS 连接、可经 Pull 兜底
- [ ] 命令可下发并回收结果

## 6. 发布

- [ ] 打标签 `vX.Y.Z`（触发 Release workflow）
- [ ] 校验 Release 产物 SHA256SUMS
