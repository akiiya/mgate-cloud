#!/usr/bin/env bash
#
# dev.sh —— 本地开发启动脚本。
#
# 开发期采用"前后端分离"调试体验最佳：
#   - 后端：go run，提供 /api 与内嵌 SPA（兜底）。
#   - 前端：Vite dev server（5173），热更新，并把 /api 代理到后端 8080。
#
# 本脚本默认启动后端；前端 dev server 请在另一个终端单独运行（见下方提示），
# 这样两个进程的日志互不干扰、各自可独立重启。
set -euo pipefail

cd "$(dirname "$0")/.."

# 本地开发的默认配置（可在执行前用环境变量覆盖）。
export MGATE_HTTP_ADDR="${MGATE_HTTP_ADDR:-:8080}"
export MGATE_DB_PATH="${MGATE_DB_PATH:-./data/mgate-cloud.db}"
export MGATE_COOKIE_SECURE="${MGATE_COOKIE_SECURE:-false}"
export MGATE_ADMIN_USERNAME="${MGATE_ADMIN_USERNAME:-admin}"
export MGATE_ADMIN_PASSWORD="${MGATE_ADMIN_PASSWORD:-change-me}"

cat <<'TIP'
─────────────────────────────────────────────
 前端热更新请在另一个终端执行：

     npm --prefix web run dev

 然后访问： http://127.0.0.1:5173/#/login
 （Vite 会把 /api 代理到后端 :8080）

 若只想用后端内嵌的前端（需先构建一次前端）：
     访问 http://127.0.0.1:8080/#/login
─────────────────────────────────────────────
TIP

echo "==> 启动后端 (go run) ..."
exec go run ./cmd/mgate-cloud
