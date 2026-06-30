# 发版操作手册 🚀

本项目以 **Git tag 为唯一版本来源**：平时正常开发，想发版时打一个 `vX.Y.Z` tag，
其余（测试 → 多平台打包 → 创建 Release 并上传资产）全部由 GitHub Actions 自动完成。
**没有 `VERSION` 文件，不需要手改任何版本文件。**

## 核心规则

- 正式版本号 = 推送的 tag 去掉前缀 `v`（`v0.1.0` → `0.1.0`），经 ldflags 注入二进制。
- 日常 / CI 构建的版本号由 `git describe --tags` 自动派生（如 `0.1.0-rc5-3-g1a2b3c4`）。
- `main` 受保护：必须经 PR 合并，禁止直接推送 / force-push。
- 日常在 `dev`（或自 `dev` 切出的特性分支）开发；提交遵循 Conventional Commits。

## 一图流

```
dev 开发 ──PR──▶ main（CI 自动跑检查）──合并
                      │
                      ▼
              打一个新的 v* tag
       （命令行 push，或 GitHub「Draft a new release」新建 tag）
                      │
                      ▼
        Release workflow 自动：测试 → 多平台打包 → 建 Release + 传资产
```

> 注意：**合并到 `main` 本身不发版**。检查在 PR 阶段跑；发版只由推送 `v*` tag 触发。

## 一、发布步骤

### 1. 先把代码合进 main
1. 在 `dev` 开发并自测。
2. 提 PR 到 `main`，等 `CI` 跑绿（`go vet` / `go test` / 前端构建 / `go build`）。
3. 审批并合并。

### 2. 打 tag 触发发布（二选一，等价）

**方式 A — 命令行**

```bash
git switch main && git pull
git tag v0.1.0
git push origin v0.1.0
```

**方式 B — GitHub 网页**

1. Releases → **Draft a new release**。
2. "Choose a tag" 输入新 tag（如 `v0.1.0`）→ 选 **Create new tag: v0.1.0 on publish**。
3. Target 选 `main`。
4. （可选）写发布说明；rc/beta 记得勾选 **Set as a pre-release**（见下文「预发布」）。
5. **Publish release**。

tag 一旦推送即触发 `Release` workflow，它会：跑测试 → 构建多平台单二进制（注入版本）
→ 打包 `tar.gz` / `zip` + `SHA256SUMS` → **自动创建 / 更新 Release 并上传资产**。
约 2 分钟后资产出现在 Release 页。

> ⚠️ tag 名必须是**没用过的新名字**；复用旧 tag 不会重新触发。
> ⚠️ 测试不通过 → workflow 失败 → **不发布**（这是有意的门禁）。

## 二、版本号与预发布

- 版本号在打 tag 时一次决定，遵循 [SemVer](https://semver.org/lang/zh-CN/)。
- **预发布**（release candidate / beta）用带连字符的 tag，如 `v0.1.0-rc5`、`v1.0.0-beta1`，
  并标记为 pre-release：
  - 网页：发布时勾选 **Set as a pre-release**；
  - 命令行：`gh release edit v0.1.0-rc5 --prerelease`。
- 为什么要标 pre-release：自更新的 **stable 通道**走 GitHub 的 `releases/latest`，它会自动
  排除 pre-release。若把 rc 当正式版发布，stable 用户会被误推 rc。**正式版用不带连字符的 tag**
  （如 `v0.1.0`），它会成为 `latest`。

## 三、发布说明（Release Notes）

`Release` workflow 用 `generate_release_notes: true` + `append_body: true`：

- **不写说明**：自动生成 “What's Changed”（自上个 tag 以来的 PR 列表 + Full Changelog）。
- **手写说明**：你写的内容**保留在上方**，自动说明**追加在下方**（不会被覆盖）。

### 推荐姿势（写「正经」说明最稳）

> 先打 tag、**不写说明** → 等 workflow 跑完（自动生成 What's Changed）→ 再去网页 **Edit**
> 补上你的版本亮点。编辑 Release **不会**触发流水线，内容稳定保留。

也可在发布时直接手写（靠 `append_body` 拼接：手写在上、自动在下）。

> 小坑：往说明里贴含 ```` ``` ```` 代码块的内容时，注意别让外层粘贴被代码围栏截断。

## 四、重新构建 / 补发资产

同一 tag 的 Release 已存在、但想重跑打包（如资产缺失）：

- Actions → `Release` → **Run workflow** → 填要重跑的 tag（`workflow_dispatch` 输入）。
- 或新建一个更高版本的 tag 重新发布（版本号 tag 不要复用）。

## 五、校验与本地打包

**使用者校验下载**（在含压缩包与 `SHA256SUMS` 的目录）：

```bash
sha256sum -c SHA256SUMS
tar -xzf mgate-cloud_<版本>_linux_amd64.tar.gz
./mgate-cloud            # 首次启动进入 Setup
```

**本地预演打包**（不真正发版，产物在 `dist/`）：

```bash
bash scripts/release.sh   # 测试 + 前端 + 多平台压缩包 + SHA256SUMS
bash scripts/build.sh     # 仅构建本机单二进制
make build                # 等价的 make 目标
```

本地构建版本号由 `git describe` 自动派生；也可 `VERSION=0.1.0 bash scripts/release.sh` 覆盖。

## 六、产物清单

每次 Release 附带：

| 平台 | 资产 |
|------|------|
| Linux x86_64 | `mgate-cloud_<版本>_linux_amd64.tar.gz` |
| Linux arm64 | `mgate-cloud_<版本>_linux_arm64.tar.gz` |
| Windows x86_64 | `mgate-cloud_<版本>_windows_amd64.zip` |
| 校验和 | `SHA256SUMS`（对压缩包计算） |

二进制内嵌前端，单文件即可运行。

## 七、相关文件

- `.github/workflows/release.yml` —— tag 触发的发布流水线。
- `.github/workflows/ci.yml` —— `dev` / PR 的检查流水线。
- `scripts/release.sh` / `scripts/build.sh` / `Makefile` —— 构建与打包。
- `internal/version/version.go` —— `Version` 变量（由 ldflags 注入）。

## 常见问题

- **打了 tag 没触发？** 确认 tag 形如 `v*` 且是新名字；查看 Actions 是否有运行。
- **资产没出现？** workflow 仍在跑或失败了；去 Actions 看 `Release` 运行日志。
- **手写说明被覆盖？** 确认 workflow 含 `append_body: true`；或采用「先发后编辑」姿势。
- **stable 用户被推了 rc？** 该 rc 没标 pre-release；用 `gh release edit <tag> --prerelease` 补标。
