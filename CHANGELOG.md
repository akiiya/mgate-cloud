# 更新日志

本项目遵循语义化版本（SemVer）。日期为 UTC。

## v0.1.0-rc3 — 发布候选 3

产品体验改版（Phase 8）：从"工程后台"升级为更现代的云产品界面，不改命令协议与数据库 schema。

### ✨ 新增

- **三套主题**：浅色 / 深色 / 跟随系统，默认跟随系统，手动切换后持久化；首屏无闪烁。
- **全新布局**：左侧主导航 + 顶栏 + 全宽内容区，移动端抽屉式导航。
- **产品化的设备操作**：白名单动作以「热点 / 上网 / 加速 / 网关 / 诊断」分类陈列，
  普通用户看到的是可读标题与说明，而非裸 action 名；带参操作（连接 WiFi / 切换节点）走受控表单。
- **结果人话化**：命令结果翻译成一句话概述 + 处置建议；原始 stdout/stderr/JSON 收进「高级详情」。
- **新增页面**：独立「更新」页与「设置 / 系统」页（外观、版本、运行模式、安全边界）。
- **只读系统信息接口**：`GET /api/admin/system`（版本 / 模式 / 更新通道 / Cookie Secure）。

### 🎨 体验

- 统一的加载 / 空 / 错误 / 离线 / 进行中状态；统一信息条（成功/警告/错误/提示）。

### 🔒 安全

- 安全边界不变：无 `os/exec` / 远程 shell，仅经 agent 下发白名单动作；静态安全测试照常通过。

## v0.1.0-rc2 — 发布候选 2

发布工程与首次启动体验硬化，不改变命令协议与数据库 schema。

### ✨ 新增

- **无配置启动**：首次运行进入 Setup 页面（`/#/setup`），完成后生成 `config.yaml`。
  配置优先级 **环境变量 > config.yaml > 默认值**；支持 `MGATE_CONFIG` 指定路径。
- **检查更新 / 自更新**：Dashboard「检查更新」卡片；`/api/admin/update/check` 与 `/api/admin/update/apply`。
  从 GitHub Releases 下载、校验 SHA256、仅替换二进制；不执行任何脚本/外部命令。
- **运维就绪探测**：`/api/readyz`（数据库可用才就绪）。

### 📦 发布工程

- Release 资产改为标准压缩包：`*_linux_amd64.tar.gz` / `*_linux_arm64.tar.gz` / `*_windows_amd64.zip`，
  内含二进制 + README/CHANGELOG/LICENSE + `deploy/` + `docs/`；`SHA256SUMS` 对压缩包计算。
- GitHub Actions：`dev`/PR 跑 CI；`main` 合并后自动测试、按 `VERSION` 打 tag、打包并发布（已存在 tag 则跳过，不覆盖）。

### 🔒 安全

- Setup 仅保存口令哈希（不写明文），`app_secret` 可自动生成；`config.yaml` 收紧 0600。
- 自更新严格校验 SHA256、保留 `.bak` 可回滚；无 `os/exec` / 远程 shell。

### 🧭 分支流程

- `main` 锁定，开发在 `dev`，经 GitHub 手动合并到 `main` 触发发布。

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
