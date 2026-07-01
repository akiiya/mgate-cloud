#!/usr/bin/env bash
#
# mgate-cloud Linux 卸载脚本（与 install.sh 对称）。
#
# 默认：停止并禁用服务 → 删除 systemd 单元与二进制，但【保留数据与配置】（含 app_secret），便于日后重装。
# 加 --purge 才【完全卸载】：连同数据、配置与系统用户一并删除（不可恢复）。
#
# 用法：
#   curl -fsSL https://raw.githubusercontent.com/akiiya/mgate-cloud/main/scripts/uninstall.sh | sudo bash
#   curl -fsSL https://raw.githubusercontent.com/akiiya/mgate-cloud/main/scripts/uninstall.sh | sudo bash -s -- --purge
#
#   --purge        完全卸载（含数据/配置/用户）；也可用环境变量 PURGE=1
#   --yes | -y     跳过二次确认（供自动化）
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

# 环境变量 PURGE=1/true/yes 等价于 --purge（先捕获环境值，再重置为布尔）。
_purge_env="${PURGE:-}"
PURGE=false
case "$_purge_env" in 1 | true | TRUE | yes | YES) PURGE=true ;; esac
ASSUME_YES=false

for arg in "$@"; do
	case "$arg" in
	--purge | --all) PURGE=true ;;
	--keep-data) PURGE=false ;; # 默认即保留数据；保留此别名以兼容
	-y | --yes) ASSUME_YES=true ;;
	-h | --help)
		grep -E '^#( |$)' "$0" 2>/dev/null | sed 's/^#\{0,1\} \{0,1\}//' || true
		exit 0
		;;
	*) die "未知参数：$arg（-h 查看用法）" ;;
	esac
done

[ "$(id -u)" = 0 ] || die "请用 root 运行：sudo bash uninstall.sh"
[ "$(uname -s)" = Linux ] || die "本脚本仅支持 Linux"

# --- 二次确认 ---
if [ "$ASSUME_YES" != true ]; then
	if [ "$PURGE" = true ]; then
		prompt="将卸载 mgate-cloud，并【删除全部数据 / 配置 / 用户】，不可恢复。继续？[y/N] "
	else
		prompt="将卸载 mgate-cloud（保留数据与配置，便于重装）。继续？[y/N] "
	fi
	if [ -r /dev/tty ]; then
		read -r -p "$prompt" ans </dev/tty || ans=""
	else
		die "非交互环境请加 --yes 确认"
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
if [ "$PURGE" = true ]; then
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
	getent group "$SVC_GROUP" >/dev/null 2>&1 && groupdel "$SVC_GROUP" 2>/dev/null || true
else
	warn "已保留：$DATA_DIR（数据）、$ENV_DIR（配置，含 app_secret）与用户 $SVC_USER"
	warn "日后重跑 install.sh 即可在原数据上恢复运行；如需彻底清除请加 --purge。"
fi

echo
log "mgate-cloud 已卸载"
[ "$PURGE" != true ] && echo "（数据与配置已保留）"
exit 0
