#!/usr/bin/env bash
#
# mgate-cloud Linux 卸载脚本（与 install.sh 对称）。
#
# 作用：停止并禁用服务 → 删除 systemd 单元 → 删除二进制 → 删除系统用户/组，
#       并按需删除或保留数据与配置。
#
# 用法：
#   sudo bash scripts/uninstall.sh              # 完全卸载（含数据/配置/用户，会二次确认）
#   sudo bash scripts/uninstall.sh --keep-data  # 保留数据与配置（含 app_secret）与用户，便于日后重装
#   sudo bash scripts/uninstall.sh --yes        # 跳过确认（供自动化）
#
# 也可用环境变量：KEEP_DATA=1 等价于 --keep-data。
set -euo pipefail

SVC=mgate-cloud
UNIT=/etc/systemd/system/mgate-cloud.service
BIN_DIR=/opt/mgate-cloud
ENV_DIR=/etc/mgate-cloud
DATA_DIR=/var/lib/mgate-cloud
SVC_USER=mgate
SVC_GROUP=mgate

log() { printf '\033[1;34m==>\033[0m %s\n' "$*"; }
warn() { printf '\033[1;33m[!]\033[0m %s\n' "$*"; }
die() {
	printf '\033[1;31m[x]\033[0m %s\n' "$*" >&2
	exit 1
}

usage() {
	sed -n '2,17p' "$0" 2>/dev/null | sed 's/^# \{0,1\}//' || true
}

# 归一化 KEEP_DATA 环境变量（1/true/yes 视为真）。先捕获环境值，再重置为布尔。
_keep_env="${KEEP_DATA:-}"
KEEP_DATA=false
case "$_keep_env" in 1 | true | TRUE | yes | YES) KEEP_DATA=true ;; esac
ASSUME_YES=false

for arg in "$@"; do
	case "$arg" in
	--keep-data) KEEP_DATA=true ;;
	-y | --yes) ASSUME_YES=true ;;
	-h | --help)
		usage
		exit 0
		;;
	*) die "未知参数：$arg（-h 查看用法）" ;;
	esac
done

[ "$(id -u)" = 0 ] || die "请用 root 运行：sudo bash uninstall.sh"
[ "$(uname -s)" = Linux ] || die "本脚本仅支持 Linux"

# --- 二次确认（非 --yes 时） ---
if [ "$ASSUME_YES" != true ]; then
	if [ "$KEEP_DATA" = true ]; then
		prompt="将停止并卸载 mgate-cloud（保留数据与配置）。继续？[y/N] "
	else
		prompt="将停止并卸载 mgate-cloud，并【删除全部数据 / 配置 / 用户】，不可恢复。继续？[y/N] "
	fi
	if [ -r /dev/tty ]; then
		read -r -p "$prompt" ans </dev/tty || ans=""
	else
		die "非交互环境请加 --yes 确认（可配合 --keep-data 保留数据）"
	fi
	case "$ans" in y | Y | yes | YES) ;; *)
		echo "已取消，未做任何更改。"
		exit 0
		;;
	esac
fi

# --- 停止并禁用服务 ---
if systemctl list-unit-files 2>/dev/null | grep -q "^${SVC}\.service" || [ -f "$UNIT" ]; then
	log "停止并禁用服务"
	systemctl disable --now "$SVC" 2>/dev/null || true
fi

# --- 删除 systemd 单元 ---
if [ -f "$UNIT" ]; then
	log "删除 systemd 单元"
	rm -f "$UNIT"
	systemctl daemon-reload
	systemctl reset-failed "$SVC" 2>/dev/null || true
fi

# --- 删除二进制 ---
if [ -d "$BIN_DIR" ]; then
	log "删除二进制目录 $BIN_DIR"
	rm -rf "$BIN_DIR"
fi

# --- 数据 / 配置 / 用户 ---
if [ "$KEEP_DATA" = true ]; then
	warn "按要求保留：$DATA_DIR（数据）、$ENV_DIR（配置，含 app_secret）与用户 $SVC_USER"
	warn "日后重跑 install.sh 即可在原数据上恢复运行。"
else
	if [ -d "$ENV_DIR" ]; then
		log "删除配置目录 $ENV_DIR"
		rm -rf "$ENV_DIR"
	fi
	if [ -d "$DATA_DIR" ]; then
		log "删除数据目录 $DATA_DIR"
		rm -rf "$DATA_DIR"
	fi
	if id "$SVC_USER" >/dev/null 2>&1; then
		log "删除系统用户 $SVC_USER"
		userdel "$SVC_USER" 2>/dev/null || warn "删除用户 $SVC_USER 失败（可稍后手动 userdel）"
	fi
	if getent group "$SVC_GROUP" >/dev/null 2>&1; then
		groupdel "$SVC_GROUP" 2>/dev/null || true
	fi
fi

echo
log "mgate-cloud 已卸载"
[ "$KEEP_DATA" = true ] && echo "（数据与配置已保留）"
exit 0
