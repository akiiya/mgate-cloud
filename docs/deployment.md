# 部署指南 🚀

本文描述把 mgate-cloud 部署到公网服务器（含生产硬化）的方式。

> 现成模板见 `deploy/`：`mgate-cloud.service`（systemd）、`mgate-cloud.env.example`（环境变量）、
> `Caddyfile.example`、`nginx.conf.example`。安全边界详见 [security.md](security.md)。

## 获取产物

**方式一：下载 Release 压缩包**（推荐）

从 GitHub Release 下载对应平台压缩包并校验：

```bash
sha256sum -c SHA256SUMS
tar -xzf mgate-cloud_<版本>_linux_amd64.tar.gz   # Windows 为 .zip
```

压缩包含二进制 + README/CHANGELOG/LICENSE + `deploy/` 模板 + `docs/`。
资产格式与分支流程见 [release-assets.md](release-assets.md)。

**方式二：本地构建**

```bash
bash scripts/build.sh   # 产物：dist/mgate-cloud（自包含，已内嵌前端）
```

二进制使用纯 Go 的 SQLite 驱动（`CGO_ENABLED=0`），无需系统 libsqlite3，便于分发与最小化镜像。

## 初始化方式

二选一：

- **Setup 页面（零配置）**：直接运行二进制 → 浏览器进入 `/#/setup` 完成初始化并生成 `config.yaml`。
  见 [setup.md](setup.md)。
- **环境变量 / 预置 config.yaml**：如下「运行」小节，适合自动化部署。

配置优先级：**环境变量 > config.yaml > 默认值**。

## 运行

```bash
MGATE_MODE=prod \
MGATE_HTTP_ADDR=127.0.0.1:8080 \
MGATE_DB_PATH=/var/lib/mgate-cloud/mgate-cloud.db \
MGATE_BASE_URL=https://cloud.example.com \
MGATE_COOKIE_SECURE=true \
MGATE_TRUST_PROXY_HEADERS=true \
MGATE_ADMIN_USERNAME=admin \
MGATE_ADMIN_PASSWORD='<强口令>' \
MGATE_SESSION_TTL_HOURS=168 \
MGATE_APP_SECRET='<固定的高强度随机串>' \
./dist/mgate-cloud
```

要点：

- **生产务必 `MGATE_MODE=prod`**：此时 `MGATE_APP_SECRET` 为空将**拒绝启动**（安全硬约束）。
- **公网部署务必 `MGATE_COOKIE_SECURE=true`**，使会话 Cookie 带 `Secure`。
- 仅当置于**可信反代**之后才设 `MGATE_TRUST_PROXY_HEADERS=true`（否则客户端可伪造来源 IP）。
- `MGATE_ADMIN_*` 仅在系统**首次**（无任何管理员）时创建账户；已存在则忽略，幂等。
- 首次创建成功后，建议从环境中移除 `MGATE_ADMIN_PASSWORD`，避免长期驻留。
- 口令与 `MGATE_APP_SECRET` 明文不会进入日志（启动日志仅显示 `(set)`）。

### ⚠️ MGATE_APP_SECRET（设备码签名密钥）

- **生产环境必须显式配置，且固定、足够随机**（例如 `openssl rand -base64 32`）。
- 为空时进程会**生成临时密钥并打印告警**：临时密钥每次重启都不同，会导致
  **此前签发的、尚未使用的设备码全部失效**（签名无法通过校验）。
- 多实例部署时，所有实例必须使用**相同**的 `MGATE_APP_SECRET`，否则跨实例无法校验设备码。
- 该密钥泄露等同于可伪造设备码，请按机密管理（环境变量 / 密钥管理服务）。

## 反向代理 / Cloudflare

mgate-cloud 自身只监听 HTTP。生产建议置于 TLS 终结之后（Cloudflare 443 或本机反代）：

- 由前端代理终结 HTTPS，回源到 `MGATE_HTTP_ADDR`。
- 透传真实客户端 IP 到 `X-Forwarded-For`（审计会取首段）。
- 保证回源为同源路径（`/api/*` 与 `/` 同域），cookie 与 CSRF 才能正常工作。

### WebSocket（WSS）

Phase 3 起 agent 经 `wss://<host>/api/agent/ws` 主动连接：

- Cloudflare 默认支持 WebSocket 透传；自建反代需放行 `Upgrade` / `Connection` 头。
- 代理的空闲超时应**大于心跳间隔**（默认 25s），否则连接会被中间层提前断开。
- WebSocket 鉴权基于 `Authorization: Bearer <device_token>`（非 cookie），与浏览器跨站凭据无关。
- `MGATE_BASE_URL` 决定 enroll 响应里下发给 agent 的 `gateway` / `ws_url` / `pull_url`，公网部署应设为对外地址。

### HTTPS Pull（兜底）

Phase 5 起 agent 可经 `POST /api/agent/pull` 兜底轮询（无需长连接）：

- 普通 HTTPS，无特殊代理要求；请求体上限 `MGATE_PULL_MAX_BODY_BYTES`（默认 128 KiB）。
- 与 WS 共用 device token 鉴权与命令 lease，互不重复投递。
- Pull 间隔由 `MGATE_PULL_DEFAULT_INTERVAL_SEC` 建议（响应 `next_pull_after_sec`）。

## 反向代理模板

- **Caddy**（自动 HTTPS + 原生 WS 透传）：见 `deploy/Caddyfile.example`。
- **Nginx**（TLS + `Upgrade` 透传）：见 `deploy/nginx.conf.example`。
- **Cloudflare**：在 443 前置即可，默认支持 WebSocket 透传；回源指向上述反代或直连 `MGATE_HTTP_ADDR`。
  经 Cloudflare 时真实 IP 在 `CF-Connecting-IP`，需配合 `MGATE_TRUST_PROXY_HEADERS=true`。

## 数据与备份

- 数据库默认单文件，外加 WAL 衍生文件：`*.db`、`*.db-shm`、`*.db-wal`。
- 推荐用脚本在线备份（一致性快照，不阻塞写）：

```bash
bash scripts/backup.sh /var/lib/mgate-cloud/mgate-cloud.db /var/backups/mgate-cloud
```

- 该脚本优先用 `sqlite3 .backup`；无 sqlite3 时回退为 WAL checkpoint + 复制（建议低写入时段）。
- **恢复**：停止服务 → 用备份覆盖 `MGATE_DB_PATH` 指向的文件（回退方式需连同 `-wal`/`-shm`）→ 启动。
- 确保 `MGATE_DB_PATH` 所在目录可写；进程启动会自动创建该目录。

## systemd

直接使用模板（含安全加固 `NoNewPrivileges` / `ProtectSystem=strict` 等）：

```bash
sudo cp dist/mgate-cloud /opt/mgate-cloud/mgate-cloud
sudo install -d /etc/mgate-cloud /var/lib/mgate-cloud
sudo cp deploy/mgate-cloud.env.example /etc/mgate-cloud/mgate-cloud.env   # 修改其中的 secret
sudo chmod 600 /etc/mgate-cloud/mgate-cloud.env
sudo cp deploy/mgate-cloud.service /etc/systemd/system/
sudo systemctl daemon-reload && sudo systemctl enable --now mgate-cloud
```

## 健康检查 / 就绪探测

```bash
curl -fsS http://127.0.0.1:8080/api/healthz   # 进程存活
# {"ok":true,"data":{"status":"ok"}}

curl -fsS http://127.0.0.1:8080/api/readyz    # 数据库可用才就绪（否则 503）
# {"ok":true,"data":{"status":"ready"}}
```

- 存活探测用 `/api/healthz`；负载均衡/编排的就绪探测用 `/api/readyz`。

## 安全提醒

- mgate-cloud 是控制面，不 SSH、不执行 shell、不提供 raw exec、不拼接 mgate.sh 命令行。
- 命令通道（WS + Pull）：cloud 仅校验 action/参数并下发 JSON，**不执行命令**；真正执行是 agent 的职责。
  命令载荷无 shell/cmd/script/args/raw。
- Phase 5 新增 HTTPS Pull 兜底：`POST /api/agent/pull` 与 WS 共用鉴权与 lease，互不重复投递。
- `capabilities` 仅记录，不用于下发命令。
- enroll 与 WebSocket 接口公开但不下放任何控制能力，信任分别建立在"有效一次性设备码"
  与"有效长期 device token"之上。
- 命令结果 stdout/stderr/result 落库有大小上限并截断；审计只记摘要，不记大输出与 token。
