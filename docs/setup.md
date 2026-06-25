# 无配置启动与初始化 🧭

mgate-cloud 支持**零配置启动**：首次运行若没有配置文件、也没有足够的环境变量，
会进入 **Setup 模式**，引导你在浏览器中完成初始化并生成 `config.yaml`。

## 启动与配置优先级

配置来源（优先级从高到低）：

1. **环境变量**（`MGATE_*`）
2. **配置文件** `config.yaml`
3. 内置默认值

配置文件路径解析：

- `MGATE_CONFIG` 指定的路径（最高优先）
- 否则当前目录 `./config.yaml`
- 否则（非 Windows）`/etc/mgate-cloud/config.yaml`
- 都不存在时，默认写入目标为 `./config.yaml`

## 进入 Setup 模式的条件

同时满足：**无配置文件** + **未提供 bootstrap 管理员环境变量** + **数据库中尚无管理员**。

> 若你用环境变量提供了 `MGATE_ADMIN_USERNAME` / `MGATE_ADMIN_PASSWORD`，或已有 `config.yaml`，
> 则按既有方式正常启动，不进入 Setup（向后兼容）。

## Setup 流程

1. 直接启动二进制（无需任何配置）：

   ```bash
   ./mgate-cloud
   ```

2. 浏览器访问服务地址，自动跳转到 `/#/setup`。
3. 填写表单（尽量少填）：
   - **管理员密码**（必填，至少 8 位）+ 确认
   - 管理员用户名（默认 `admin`）
   - 运行模式 `mode`（默认 `dev`，可选 `prod`）
   - 对外访问地址 `base_url`（默认取当前浏览器地址）
   - 高级（可选）：监听地址、数据库路径、是否信任反代头、`app_secret`（留空自动生成）
4. 提交后：创建管理员、写入 `config.yaml`、切换到正常模式（无需重启即可登录）。
5. 若更改了 `mode` / `app_secret` / `cookie_secure`，提示重启以完全生效。

Setup 模式下，除 `/#/setup`、静态页面、`/api/healthz`、`/api/readyz`、`/api/setup/*` 外，
其余 API 返回 `setup_required`，正常登录接口同样返回 `setup_required`。

## config.yaml 示例

```yaml
# mgate-cloud 配置文件（由 Setup 生成）。
# 注意：本文件含 app_secret 等敏感信息，请妥善保管、收紧权限（0600）。
# 环境变量优先级高于本文件。

http_addr: ":8080"
base_url: "https://cloud.example.com"
db_path: "/var/lib/mgate-cloud/mgate-cloud.db"
mode: "prod"
cookie_secure: true
trust_proxy_headers: true

# 设备码签名密钥：生产环境必须固定保存；丢失将导致已发设备码校验失败。
app_secret: "<高强度随机串>"

# 管理员：仅保存口令哈希（bcrypt），绝不保存明文。
admin_username: "admin"
admin_password_hash: "$2a$12$..."

# 更新检查（GitHub Releases）。
update_check_enabled: true
update_channel: "stable"
github_repo: "akiiya/mgate-cloud"
```

## Setup API

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/api/setup/status` | 返回是否需要 setup 及表单默认值 |
| POST | `/api/setup/complete` | 完成初始化（创建管理员 + 写 config.yaml） |

`complete` 要求：严格 JSON、请求体大小受限、字段校验；完成后写审计 `system.setup.completed`，
且 setup 完成后不可重复调用。

## 安全要点 🔐

1. **绝不保存明文密码**：Setup 只把 bcrypt 口令哈希写入 `config.yaml`。
2. `app_secret` 留空时自动生成强随机值并落盘；**生产模式必须有固定 app_secret**。
3. 生成的 `config.yaml` 权限收紧到 `0600`（Windows 权限语义有限，请自行确保文件访问控制）。
4. 启动日志与审计**绝不**输出 secret / password / token 明文。
