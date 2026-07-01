# mgate-cloud ☁️

公网部署的 **mgate 设备管理控制面**。版本信息见 [CHANGELOG.md](CHANGELOG.md) 与发布页。

管理员经浏览器集中管理随身 WiFi 设备；设备上的 `mgate-agent` 主动连接 cloud
（WebSocket 主通道 + HTTPS Pull 兜底），cloud 经**白名单 action** 下发指令并回收结果。

> **安全边界（硬约束）**：cloud 是控制面，不是执行器。它**不** SSH、不执行 shell、
> 不拼接 `mgate.sh`、不提供 raw exec；只校验白名单 action 并下发 JSON。真正执行是 agent 的职责。

## ✨ 能力一览

- 🔐 管理后台：登录/登出、Session Cookie、CSRF、审计日志、现代化 SPA（hash 路由，单二进制内嵌）。
- 📟 设备身份：设备管理、一次性设备码、agent enroll、长期凭证（仅存哈希）。
- 🔌 Agent 连接：WebSocket Hub、在线状态、心跳、状态上报。
- 🎛️ 命令通道：白名单 action、严格参数校验、ack/result 回放、超时/重试/过期清理。
- 📥 Pull 兜底：HTTPS 轮询、离线命令队列；WS 与 Pull 共用 lease，不重复投递。
- 🗄️ 存储：SQLite + WAL，零外部中间件；单可执行文件即可运行。

## 🚀 快速开始

前置：**Go ≥ 1.26**、**Node ≥ 18**。

```bash
# 一键构建（前端 + 内嵌单二进制，注入版本号）
bash scripts/build.sh

# 零配置启动：直接运行，浏览器访问后进入 /#/setup 完成初始化（生成 config.yaml）
./dist/mgate-cloud
# 打开 http://127.0.0.1:8080/  → 自动跳转 /#/setup

# 或用环境变量直接创建管理员（跳过 Setup，向后兼容）
MGATE_ADMIN_USERNAME=admin MGATE_ADMIN_PASSWORD=change-me ./dist/mgate-cloud
```

> 首次初始化（无配置启动）流程见 [docs/setup.md](docs/setup.md)。配置优先级：**环境变量 > config.yaml > 默认值**。

或用 Make：`make build`、`make test`、`make run`、`make help`。

## 🐧 一键安装（Linux 生产部署）

自动完成：下载校验二进制 → 创建系统用户与数据目录 → 生成运行配置 → 安装并启动 systemd 服务。

```bash
curl -fsSL https://raw.githubusercontent.com/akiiya/mgate-cloud/main/scripts/install.sh | sudo bash
```

装好后服务以 `mgate` 用户运行、监听 `127.0.0.1:8080`（请在其前置 Caddy/Nginx 终结 HTTPS）。
浏览器打开 `https://你的域名/#/setup` 创建管理员即可。**升级**：重跑同一命令（保留配置与数据）。

非交互初始化（直接建管理员、指定对外地址 / 版本，可选）：

```bash
curl -fsSL https://raw.githubusercontent.com/akiiya/mgate-cloud/main/scripts/install.sh \
  | sudo MGATE_BASE_URL=https://cloud.example.com \
         MGATE_ADMIN_USERNAME=admin MGATE_ADMIN_PASSWORD='强口令' bash
```

| 环境变量 | 作用 |
|------|------|
| `MGATE_BASE_URL` | 对外访问地址（`https://` 时自动开启 Secure Cookie） |
| `MGATE_ADMIN_USERNAME` / `MGATE_ADMIN_PASSWORD` | 直接创建管理员（否则走浏览器 Setup） |
| `VERSION` | 指定版本（默认最新 Release） |

卸载：

```bash
sudo bash scripts/uninstall.sh              # 完全卸载（含数据/配置/用户，会二次确认）
sudo bash scripts/uninstall.sh --keep-data  # 保留数据与配置，便于日后重装
```

> 反向代理示例见 [`deploy/Caddyfile.example`](deploy/Caddyfile.example) / [`deploy/nginx.conf.example`](deploy/nginx.conf.example)；生产要点见 [docs/deployment.md](docs/deployment.md)。

分项验收命令：

```bash
go test ./...                      # 含禁止远程 shell 的静态安全测试
go vet ./...
npm --prefix web run build
go build -o mgate-cloud ./cmd/mgate-cloud
```

## ⚙️ 关键环境变量

| 变量 | 默认 | 说明 |
|------|------|------|
| `MGATE_CONFIG` | （空） | 指定 config.yaml 路径（优先于 ./config.yaml、/etc/mgate-cloud/config.yaml） |
| `MGATE_MODE` | `dev` | `dev`/`test`/`prod`；prod 下 `MGATE_APP_SECRET` 为空将拒绝启动 |
| `MGATE_HTTP_ADDR` | `:8080` | 监听地址 |
| `MGATE_DB_PATH` | `./data/mgate-cloud.db` | SQLite 路径（目录自动创建） |
| `MGATE_BASE_URL` | `http://127.0.0.1:8080` | 对外地址（下发给 agent 的 gateway/ws/pull） |
| `MGATE_COOKIE_SECURE` | `false` | HTTPS 部署置 `true` |
| `MGATE_APP_SECRET` | （空） | 设备码 HMAC 密钥；生产必须固定且足够随机 |
| `MGATE_ADMIN_USERNAME` / `MGATE_ADMIN_PASSWORD` | （空） | 首次 bootstrap 管理员 |

完整变量见 [docs/deployment.md](docs/deployment.md)。

## 🩺 运维端点

- `GET /api/healthz` — 进程存活。
- `GET /api/readyz` — 数据库可用才就绪（否则 503）。

## 📚 文档

| 文档 | 内容 |
|------|------|
| [docs/architecture.md](docs/architecture.md) | 架构、分层、数据模型与设计要点 |
| [docs/development.md](docs/development.md) | 本地开发、接口速览、模拟器、手动验证 |
| [docs/protocol.md](docs/protocol.md) | 设备码与 enroll 协议 |
| [docs/agent-ws.md](docs/agent-ws.md) | Agent WebSocket 协议 |
| [docs/agent-pull.md](docs/agent-pull.md) | HTTPS Pull 兜底协议 |
| [docs/commands.md](docs/commands.md) | 白名单命令、状态机、结果存储 |
| [docs/security.md](docs/security.md) | 安全边界与硬化 |
| [docs/setup.md](docs/setup.md) | 无配置启动与初始化、config.yaml |
| [docs/update.md](docs/update.md) | 检查更新 / 自更新 |
| [docs/deployment.md](docs/deployment.md) | 生产部署（systemd / 反代 / 备份） |
| [docs/releasing.md](docs/releasing.md) | 发版操作手册（打 tag 发布、Release Notes、校验） |
| [docs/release-assets.md](docs/release-assets.md) | 发布资产格式与压缩包内容 |
| [docs/release-checklist.md](docs/release-checklist.md) | 发布清单 |
| [CHANGELOG.md](CHANGELOG.md) | 版本变更 |

## 📦 单二进制

`go build` 通过 `//go:embed all:dist` 把前端打入可执行文件：构建后**无需** `web/dist`
目录即可运行；删除源码 `web/dist` 后，已编译二进制仍能提供页面。

## 🚫 刻意不包含

无 SSH / raw exec / 远程 shell；无 Telegram Bot；无 Redis/MQTT/Postgres；不做多实例/集群。
`capabilities` 仅记录，不作为可下发命令列表。

## 📄 License

[MIT](LICENSE) © 2026 mgate-cloud authors
