#!/usr/bin/env bash
#
# build.sh —— 构建可发布的单二进制。
#
# 串联前端构建与 Go 构建：前端产物输出到 web/dist，再由 Go embed 内嵌，
# 最终在 dist/ 下得到自包含、无需外部静态资源的可执行文件。
#
# 用法：
#   ./scripts/build.sh
#   ./dist/mgate-cloud
set -euo pipefail

# 切到仓库根目录，保证相对路径稳定。
cd "$(dirname "$0")/.."

# 版本号取自 VERSION 文件，经 ldflags 注入二进制（启动日志可见）。
VERSION="$(tr -d ' \t\n\r' < VERSION 2>/dev/null || echo dev)"
LDFLAGS="-s -w -X mgate-cloud/internal/version.Version=${VERSION}"

echo "==> [1/3] 安装前端依赖"
npm --prefix web install

echo "==> [2/3] 构建前端（输出到 web/dist）"
npm --prefix web run build
# vite emptyOutDir 会清空 web/dist 并删除占位文件；重建它以保证可提交且 fresh clone 可编译。
touch web/dist/.gitkeep

echo "==> [3/3] 构建 Go 单二进制（内嵌前端资源, version=${VERSION}）"
mkdir -p dist
# CGO_ENABLED=0：使用纯 Go 的 SQLite 驱动，产物自包含、易于跨平台分发。
CGO_ENABLED=0 go build -trimpath -ldflags "${LDFLAGS}" -o dist/mgate-cloud ./cmd/mgate-cloud

echo "==> 完成 ✅  产物: dist/mgate-cloud (version=${VERSION})"
