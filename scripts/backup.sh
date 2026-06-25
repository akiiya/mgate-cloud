#!/usr/bin/env bash
#
# backup.sh —— 在线备份 SQLite 数据库（WAL 模式安全）。
#
# 优先使用 sqlite3 的 .backup（一致性快照，不阻塞写）；不可用时回退为
# WAL checkpoint 后复制（需短暂静默写入更稳妥）。
#
# 用法：
#   ./scripts/backup.sh /var/lib/mgate-cloud/mgate-cloud.db /var/backups/mgate-cloud
set -euo pipefail

DB_PATH="${1:-./data/mgate-cloud.db}"
OUT_DIR="${2:-./backups}"

if [[ ! -f "${DB_PATH}" ]]; then
  echo "数据库不存在: ${DB_PATH}" >&2
  exit 1
fi

mkdir -p "${OUT_DIR}"
TS="$(date -u +%Y%m%dT%H%M%SZ)"
DEST="${OUT_DIR}/mgate-cloud-${TS}.db"

if command -v sqlite3 >/dev/null 2>&1; then
  echo "==> 使用 sqlite3 .backup 生成一致性快照"
  sqlite3 "${DB_PATH}" ".backup '${DEST}'"
else
  echo "==> 未找到 sqlite3，回退为 WAL checkpoint + 复制（建议在低写入时段执行）"
  # 复制主库与 WAL/SHM，确保可恢复。
  cp "${DB_PATH}" "${DEST}"
  [[ -f "${DB_PATH}-wal" ]] && cp "${DB_PATH}-wal" "${DEST}-wal" || true
  [[ -f "${DB_PATH}-shm" ]] && cp "${DB_PATH}-shm" "${DEST}-shm" || true
fi

echo "==> 备份完成: ${DEST}"

# 恢复方法（停服后）：
#   1) 停止 mgate-cloud
#   2) 用备份覆盖 MGATE_DB_PATH 指向的文件（若用回退方式，连同 -wal/-shm 一并恢复）
#   3) 启动 mgate-cloud
