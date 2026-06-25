# 检查更新与自更新 ⬆️

mgate-cloud 提供保守的"检查更新 / 自更新"能力：从 GitHub Releases 获取最新版本，
校验后**只替换自身二进制**。它**绝不**执行下载包内的任何脚本，也**不**引入任何远程 shell 能力。

## 能力

- 后台 Dashboard「检查更新」卡片：显示当前版本、最新版本、发布时间、下载资产，提示是否有新版本。
- 有新版本时可「下载并更新」，更新前提醒**先备份数据库**。

## API

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/api/admin/update/check` | 查询 GitHub 最新 release 并与当前版本比较 |
| POST | `/api/admin/update/apply` | 下载 → 校验 SHA256 → 解压 → 备份 → 替换二进制 |

均需管理员登录；`apply` 需 CSRF。更新功能可由配置禁用。

## 配置

| 配置（YAML / 环境变量） | 默认 | 说明 |
|------|------|------|
| `update_check_enabled` / `MGATE_UPDATE_CHECK_ENABLED` | `true` | 是否启用更新检查/自更新 |
| `update_channel` / `MGATE_UPDATE_CHANNEL` | `stable` | `stable`（排除预发布）或 `rc`（含预发布） |
| `github_repo` / `MGATE_GITHUB_REPO` | `akiiya/mgate-cloud` | 更新来源仓库 |

禁用时，相关 API 返回 `update_disabled`。

## 自更新流程

1. 按当前 `OS/ARCH` 选择 release 压缩包：`mgate-cloud_<版本>_<os>_<arch>.{tar.gz|zip}`。
2. 下载压缩包与 `SHA256SUMS`。
3. **校验 SHA256**（不一致立即中止）。
4. 解压到临时 staging 目录，**仅提取 mgate-cloud 二进制**（忽略其它文件）。
5. 备份当前二进制为 `<bin>.bak`。
6. 原子替换当前二进制（同目录临时文件 + rename）。
7. 写审计 `system.update.applied`，返回"需要重启"。

systemd 部署下，重启进程即可加载新版本（`systemctl restart mgate-cloud`）。

## 安全边界 🔒

- **不使用** `os/exec` / `exec.Command` / `bash` / `sh`，**不执行任何外部命令**。
- **不执行**下载包内的任何脚本；即使压缩包内含脚本也不会运行。
- **只允许替换 mgate-cloud 二进制**，不写入其它路径。
- SHA256 **校验失败必须中止**；失败时保留 `.bak` 以便回滚。
- 更新前**务必备份数据库**（前端有明确提醒）。

## Windows 注意 ⚠️

Windows 无法替换正在运行的 `.exe`。此时自更新会：

- 通过校验后把新二进制下载到 `<bin>.new`（旧版本备份为 `<bin>.bak`）；
- 返回 `needs_manual = true` 与提示；
- 请**停止服务**后用 `.new` 覆盖当前 `mgate-cloud.exe`，再启动。

## 回滚

如更新后异常，停止服务并用 `<bin>.bak` 覆盖回当前二进制即可恢复到旧版本。
