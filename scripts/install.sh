#!/usr/bin/env bash
#
# mgate-cloud Linux 一键安装 / 升级脚本。
#
# 作用：下载对应架构的发布版二进制 → 校验 SHA256 → 安装到 /opt → 创建系统用户与数据目录
#       → 生成运行配置（含固定 app_secret）→ 安装并启动 systemd 服务。
#
# 用法：
#   curl -fsSL https://raw.githubusercontent.com/akiiya/mgate-cloud/main/scripts/install.sh | sudo bash
#
# 初始化（创建管理员、对外地址等）一律在浏览器的 Setup 页完成，脚本不预置这些参数。
#
# 可选环境变量：
#   VERSION  指定版本（默认安装最新 Release，如 v0.1.0）
#   REPO     来源仓库（默认 akiiya/mgate-cloud）
#
# 重复执行即为“升级”：仅替换二进制并重启，保留既有配置与数据。
set -euo pipefail

REPO="${REPO:-akiiya/mgate-cloud}"
VERSION="${VERSION:-}"
SVC_USER=mgate
SVC_GROUP=mgate
BIN_DIR=/opt/mgate-cloud
BIN="$BIN_DIR/mgate-cloud"
ENV_DIR=/etc/mgate-cloud
ENV_FILE="$ENV_DIR/mgate-cloud.env"
DATA_DIR=/var/lib/mgate-cloud
UNIT=/etc/systemd/system/mgate-cloud.service

log() { printf '\033[1;34m==>\033[0m %s\n' "$*"; }
warn() { printf '\033[1;33m[!]\033[0m %s\n' "$*"; }
die() {
	printf '\033[1;31m[x]\033[0m %s\n' "$*" >&2
	exit 1
}

# 生成高熵 app_secret（优先 openssl，退回 /dev/urandom），十六进制以避免特殊字符。
gen_secret() {
	if command -v openssl >/dev/null 2>&1; then
		openssl rand -hex 32
	else
		head -c 32 /dev/urandom | od -An -tx1 | tr -d ' \n'
	fi
}

# 安全下载：强制 HTTPS（含重定向）、TLS ≥ 1.2、失败自动重试。
dl() { curl --proto '=https' --proto-redir '=https' --tlsv1.2 --retry 3 --retry-delay 2 -fSL "$@"; }

# --- 前置检查 ---
[ "$(id -u)" = 0 ] || die "请用 root 运行：curl -fsSL <url> | sudo bash"
[ "$(uname -s)" = Linux ] || die "本脚本仅支持 Linux"
for c in curl tar sha256sum systemctl install; do
	command -v "$c" >/dev/null 2>&1 || die "缺少依赖命令：$c"
done

case "$(uname -m)" in
x86_64 | amd64) ARCH=amd64 ;;
aarch64 | arm64) ARCH=arm64 ;;
*) die "不支持的架构：$(uname -m)（仅支持 amd64 / arm64）" ;;
esac

# --- 解析版本 ---
if [ -z "$VERSION" ]; then
	log "查询最新版本…"
	VERSION="$(dl -s "https://api.github.com/repos/$REPO/releases/latest" |
		grep -o '"tag_name":[[:space:]]*"[^"]*"' | head -1 | sed 's/.*"\([^"]*\)"$/\1/')"
	[ -n "$VERSION" ] || die "无法获取最新版本，请用 VERSION=vX.Y.Z 指定"
fi
VER_NUM="${VERSION#v}"
ASSET="mgate-cloud_${VER_NUM}_linux_${ARCH}.tar.gz"
BASE_DL="https://github.com/$REPO/releases/download/$VERSION"
log "准备安装 mgate-cloud $VERSION（$ARCH）"

# --- 下载 + 校验 ---
TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT
log "下载 $ASSET"
dl "$BASE_DL/$ASSET" -o "$TMP/$ASSET" || die "下载二进制失败"
dl "$BASE_DL/SHA256SUMS" -o "$TMP/SHA256SUMS" || die "下载校验和失败"
log "校验 SHA256"
(cd "$TMP" && grep " $ASSET\$" SHA256SUMS | sha256sum -c -) || die "SHA256 校验失败，已中止"
tar -xzf "$TMP/$ASSET" -C "$TMP"
[ -f "$TMP/mgate-cloud" ] || die "压缩包内未找到 mgate-cloud 二进制"

# --- 是否首次安装（决定后续提示语） ---
FRESH_INSTALL=true
[ -f "$ENV_FILE" ] && FRESH_INSTALL=false

# --- 系统用户与目录 ---
getent group "$SVC_GROUP" >/dev/null 2>&1 || { log "创建用户组 $SVC_GROUP"; groupadd --system "$SVC_GROUP"; }
if ! id "$SVC_USER" >/dev/null 2>&1; then
	log "创建系统用户 $SVC_USER"
	useradd --system --gid "$SVC_GROUP" --no-create-home \
		--shell /usr/sbin/nologin "$SVC_USER" 2>/dev/null ||
		useradd --system --gid "$SVC_GROUP" --no-create-home --shell /bin/false "$SVC_USER"
fi
install -d -m 0755 "$BIN_DIR"
install -d -m 0750 "$ENV_DIR"
install -d -m 0750 -o "$SVC_USER" -g "$SVC_GROUP" "$DATA_DIR"

# --- 安装二进制 ---
log "安装二进制到 $BIN"
install -m 0755 "$TMP/mgate-cloud" "$BIN"

# --- 运行配置（存在则保留，不覆盖 app_secret） ---
# 只写启动所需的最小项：prod 模式必须有固定 app_secret，否则拒绝启动。
# 对外地址 / Secure Cookie / 管理员等由浏览器 Setup 页配置并写入 config.yaml。
if [ "$FRESH_INSTALL" = true ]; then
	log "生成运行配置 $ENV_FILE"
	SECRET="$(gen_secret)"
	# 在子 shell 内收紧 umask 写文件，避免 umask 泄漏影响后续（如 systemd 单元）权限。
	(
		umask 077
		{
			echo "# mgate-cloud 运行配置（权限 600，含 app_secret，请妥善保管）。"
			echo "# 生成于 $(date -u '+%Y-%m-%dT%H:%M:%SZ')。其余配置在浏览器 Setup 页完成。"
			echo "MGATE_MODE=prod"
			echo "MGATE_HTTP_ADDR=127.0.0.1:8080"
			echo "MGATE_DB_PATH=$DATA_DIR/mgate-cloud.db"
			echo "MGATE_APP_SECRET=$SECRET"
		} >"$ENV_FILE"
	)
	chmod 600 "$ENV_FILE"
else
	warn "检测到已存在 $ENV_FILE：保留现有配置与数据，仅升级二进制"
fi

# --- systemd 单元（每次覆盖，保证与本脚本一致） ---
log "安装 systemd 单元 $UNIT"
cat >"$UNIT" <<EOF
[Unit]
Description=mgate-cloud device management control plane
After=network.target

[Service]
Type=simple
User=$SVC_USER
Group=$SVC_GROUP
EnvironmentFile=$ENV_FILE
ExecStart=$BIN
Restart=on-failure
RestartSec=3

# 安全加固：本服务不需要任何命令执行 / 提权能力。
NoNewPrivileges=true
ProtectSystem=strict
ProtectHome=true
PrivateTmp=true
PrivateDevices=true
ProtectKernelTunables=true
ProtectControlGroups=true
RestrictSUIDSGID=true
RestrictNamespaces=true
LockPersonality=true
MemoryDenyWriteExecute=true
ReadWritePaths=$DATA_DIR

[Install]
WantedBy=multi-user.target
EOF
chmod 0644 "$UNIT"

log "启动服务"
systemctl daemon-reload
systemctl enable --now mgate-cloud
sleep 1

if ! systemctl is-active --quiet mgate-cloud; then
	warn "服务未处于 active 状态，请查看日志：journalctl -u mgate-cloud -e"
fi

# --- 结语 ---
echo
log "安装完成：mgate-cloud $VERSION"
echo "    二进制    : $BIN"
echo "    配置      : $ENV_FILE（权限 600）"
echo "    数据目录  : $DATA_DIR"
echo "    监听       : 127.0.0.1:8080（仅本机；请在其前置 Caddy/Nginx 终结 HTTPS）"
echo
if [ "$FRESH_INSTALL" != true ]; then
	echo "已升级并重启，保留原有配置与数据。"
else
	echo "下一步：前置反向代理后，浏览器打开  https://你的域名/#/setup  创建管理员并完成初始化。"
	echo "（服务仅监听 127.0.0.1，未架反代前可用 SSH 隧道访问 127.0.0.1:8080）"
fi
echo
echo "常用命令：systemctl status mgate-cloud | journalctl -u mgate-cloud -f"
