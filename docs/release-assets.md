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
- **main**：触发 `Release` workflow ——
  1. 跑 CI（测试 + 前端）。
  2. 读取 `VERSION`。
  3. 若 `v<VERSION>` tag **不存在**：打包多平台压缩包 → 创建并推送 tag → 发布 Release（附压缩包 + `SHA256SUMS`，release notes 取 `CHANGELOG.md`）。
  4. 若 tag **已存在**：跳过发布，**绝不覆盖**已有 tag/release。
- 支持 `workflow_dispatch` 手动重跑 Release。

### 发布新版本的步骤

1. 在 `dev` 完成开发与验证。
2. 更新 `VERSION` 与 `CHANGELOG.md`（新增对应版本条目）。
3. 通过 GitHub 页面把 `dev` 合并到 `main`。
4. `main` 的 Release workflow 自动打 tag 并发布。

> 注意：升级版本号前确认 `VERSION` 与 `CHANGELOG.md` 一致；tag 一旦发布不可覆盖，
> 如需重发请提升版本号。
