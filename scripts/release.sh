#!/usr/bin/env bash
#
# release.sh —— 发布前完整构建与校验。
#
# 串联：静态检查 → 测试 → 前端构建 → Go 单二进制（注入版本）→ 生成校验和。
# 该脚本不进行任何真实发布（不打 tag、不上传），仅产出 dist/ 下可发布产物，
# 便于本地核对发布流水线的关键命令。
#
# 用法：
#   ./scripts/release.sh            # 用 VERSION 文件中的版本
#   VERSION=0.1.0-rc1 ./scripts/release.sh
set -euo pipefail

cd "$(dirname "$0")/.."

VERSION="${VERSION:-$(tr -d ' \t\n\r' < VERSION)}"
LDFLAGS="-s -w -X mgate-cloud/internal/version.Version=${VERSION}"

echo "==> 发布版本: ${VERSION}"

echo "==> [1/5] go vet"
go vet ./...

echo "==> [2/5] go test（含禁止远程 shell 的静态安全测试）"
go test ./...

echo "==> [3/5] 构建前端"
npm --prefix web install
npm --prefix web run build
# vite emptyOutDir 会清空 web/dist 并删除占位文件；重建它以保证可提交且 fresh clone 可编译。
touch web/dist/.gitkeep

echo "==> [4/5] 构建 Go 单二进制（内嵌前端）"
mkdir -p dist
CGO_ENABLED=0 go build -trimpath -ldflags "${LDFLAGS}" -o dist/mgate-cloud ./cmd/mgate-cloud

echo "==> [5/5] 生成校验和"
( cd dist && sha256sum mgate-cloud > mgate-cloud.sha256 2>/dev/null || shasum -a 256 mgate-cloud > mgate-cloud.sha256 )

echo "==> 完成 ✅"
echo "    产物: dist/mgate-cloud"
echo "    校验: dist/mgate-cloud.sha256"
echo "    版本: ${VERSION}"
