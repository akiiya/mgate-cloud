# 发布资产与分支流程 📦

## Release 资产

每次发布在 GitHub Release 提供标准压缩包（不再是裸二进制）：

| 平台 | 资产 |
|------|------|
| Linux x86_64 | `mgate-cloud_<版本>_linux_amd64.tar.gz` |
| Linux arm64 | `mgate-cloud_<版本>_linux_arm64.tar.gz` |
| Windows x86_64 | `mgate-cloud_<版本>_windows_amd64.zip` |
| 校验和 | `SHA256SUMS`（对**压缩包**计算） |

每个压缩包至少包含：

```
mgate-cloud (或 mgate-cloud.exe)
README.md
CHANGELOG.md
LICENSE
deploy/mgate-cloud.env.example
deploy/mgate-cloud.service
deploy/Caddyfile.example
deploy/nginx.conf.example
docs/deployment.md
docs/security.md
```

二进制内嵌前端，单文件即可运行。版本号经 ldflags 注入，启动日志可见。

### 校验下载

```bash
sha256sum -c SHA256SUMS         # 在含压缩包与 SHA256SUMS 的目录执行
tar -xzf mgate-cloud_<版本>_linux_amd64.tar.gz
./mgate-cloud                    # 首次启动进入 Setup
```

## 本地打包

```bash
bash scripts/release.sh         # 测试 + 前端 + 多平台压缩包 + SHA256SUMS（产出到 dist/）
```

> 本机无 `zip` 命令时会跳过 Windows zip（CI/Linux 会生成）。

## 分支与发布流程

`main` 分支已锁定，发布候选后所有开发在 `dev` 分支进行，经 PR/手动 merge 进入 `main`。

- **dev / PR**：触发 `CI` workflow —— `go vet`、`go test`（含静态安全测试）、前端构建、`go build`。**不发布**。
- **推送 `v*` tag**：触发 `Release` workflow（**以 tag 为唯一来源**）——
  1. 从 tag 解析版本号（去掉前缀 `v`），经 ldflags 注入二进制。
  2. 跑测试 + 构建前端 → 多平台打包压缩包 + `SHA256SUMS`。
  3. 创建 GitHub Release，附压缩包与校验和；发布说明由 GitHub 依据提交/PR 自动生成。
- 支持 `workflow_dispatch` 指定某个 tag 手动重跑。

### 发布新版本的步骤（无 VERSION 文件，无需手改版本号）

1. 在 `dev` 完成开发与验证。
2. 通过 GitHub 页面把 `dev` 合并到 `main`。
3. 在 `main` 打 tag 触发发布：`git tag v0.1.0 && git push origin v0.1.0`，
   或在 GitHub「Draft a new release」里直接新建 tag 并发布。

> 版本号在打 tag 时一次决定（语义化版本）；日常/本地构建版本由 `git describe` 自动派生。
> 同一版本 tag 的 Release 已存在；如需重发请用 workflow_dispatch 重跑或新建更高版本 tag。
