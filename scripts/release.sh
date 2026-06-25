#!/usr/bin/env bash
#
# release.sh —— 发布前完整构建与打包（标准压缩包）。
#
# 串联：静态检查 → 测试 → 前端构建 → 多平台单二进制（注入版本）→ 打包 tar.gz / zip
#       → 生成 SHA256SUMS（对压缩包）。
# 不进行任何真实发布（不打 tag、不上传），仅产出 dist/ 下可发布资产，便于本地核对发布流水线。
#
# 用法：
#   ./scripts/release.sh
#   VERSION=0.1.0 ./scripts/release.sh
set -euo pipefail

cd "$(dirname "$0")/.."

VERSION="${VERSION:-$(tr -d ' \t\n\r' < VERSION)}"
LDFLAGS="-s -w -X mgate-cloud/internal/version.Version=${VERSION}"

# 压缩包内附带的随附文件。
EXTRA_FILES=(
  README.md
  CHANGELOG.md
  LICENSE
  deploy/mgate-cloud.env.example
  deploy/mgate-cloud.service
  deploy/Caddyfile.example
  deploy/nginx.conf.example
  docs/deployment.md
  docs/security.md
)

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

echo "==> [4/5] 多平台打包"
rm -rf dist && mkdir -p dist

package() {
  local goos="$1" goarch="$2" ext="$3" archive="$4"
  local bin="mgate-cloud"; [ -n "${ext}" ] && bin="mgate-cloud${ext}"
  local stage="dist/stage_${goos}_${goarch}"
  rm -rf "${stage}" && mkdir -p "${stage}"

  echo "    - build ${goos}/${goarch}"
  CGO_ENABLED=0 GOOS="${goos}" GOARCH="${goarch}" \
    go build -trimpath -ldflags "${LDFLAGS}" -o "${stage}/${bin}" ./cmd/mgate-cloud
  # 保留 deploy/ 与 docs/ 的相对路径结构。
  for f in "${EXTRA_FILES[@]}"; do
    mkdir -p "${stage}/$(dirname "${f}")"
    cp "${f}" "${stage}/${f}"
  done

  case "${archive}" in
    tar.gz)
      tar -C "${stage}" -czf "dist/mgate-cloud_${VERSION}_${goos}_${goarch}.tar.gz" .
      ;;
    zip)
      if command -v zip >/dev/null 2>&1; then
        ( cd "${stage}" && zip -qr "../mgate-cloud_${VERSION}_${goos}_${goarch}.zip" . )
      else
        echo "      (跳过 zip：本机无 zip 命令；CI/Linux 会生成)"
      fi
      ;;
  esac
  rm -rf "${stage}"
}

package linux amd64 ""     tar.gz
package linux arm64 ""     tar.gz
package windows amd64 .exe zip

echo "==> [5/5] 生成 SHA256SUMS（对压缩包）"
( cd dist && (sha256sum mgate-cloud_* 2>/dev/null || shasum -a 256 mgate-cloud_*) > SHA256SUMS )

echo "==> 完成 ✅  版本 ${VERSION}"
ls -1 dist
