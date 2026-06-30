# 发布资产 📦

> 完整发版流程（打 tag、Release Notes、校验、重跑等）见 **[releasing.md](releasing.md)**。
> 本文只描述**资产格式与压缩包内容**。

## Release 资产

每次发布在 GitHub Release 提供标准压缩包（不再是裸二进制）：

| 平台 | 资产 |
|------|------|
| Linux x86_64 | `mgate-cloud_<版本>_linux_amd64.tar.gz` |
| Linux arm64 | `mgate-cloud_<版本>_linux_arm64.tar.gz` |
| Windows x86_64 | `mgate-cloud_<版本>_windows_amd64.zip` |
| 校验和 | `SHA256SUMS`（对**压缩包**计算） |

## 压缩包内容

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

二进制内嵌前端，单文件即可运行；版本号经 ldflags 注入，启动日志可见。
