#!/usr/bin/env zsh
# Panel launchd 安装/重装脚本。等幂,可重复运行。
# 用法:./scripts/install.sh
set -euo pipefail

PANEL_DIR="${HOME}/panel"
BIN_DIR="${PANEL_DIR}/bin"
BIN="${BIN_DIR}/panel-backend"
PLIST_NAME="com.kinesis.panel.plist"
PLIST_SRC="${PANEL_DIR}/scripts/${PLIST_NAME}"
PLIST_DST="${HOME}/Library/LaunchAgents/${PLIST_NAME}"
LABEL="com.kinesis.panel"
DOMAIN="gui/$(id -u)"

mkdir -p "${BIN_DIR}" "${HOME}/Library/LaunchAgents"

# 已加载则先 bootout,避免端口冲突或旧二进制残留
if launchctl print "${DOMAIN}/${LABEL}" >/dev/null 2>&1; then
    echo "==> bootout existing ${LABEL}"
    launchctl bootout "${DOMAIN}" "${PLIST_DST}" 2>/dev/null || true
    sleep 1
fi

# 端口检查 — launchd 之外的 panel 进程会让 KeepAlive 死循环
if lsof -nP -iTCP:8089 -sTCP:LISTEN >/dev/null 2>&1; then
    echo "⚠️  127.0.0.1:8089 仍被占用,请先 kill 后重跑:"
    lsof -nP -iTCP:8089 -sTCP:LISTEN
    exit 1
fi

echo "==> building panel-backend → ${BIN}"
( cd "${PANEL_DIR}/backend" && go build -o "${BIN}" . )

echo "==> rendering plist → ${PLIST_DST}"
sed -e "s|__PANEL_BIN__|${BIN}|g" \
    -e "s|__PANEL_DIR__|${PANEL_DIR}|g" \
    "${PLIST_SRC}" > "${PLIST_DST}"

echo "==> bootstrap ${DOMAIN}"
launchctl bootstrap "${DOMAIN}" "${PLIST_DST}"
launchctl enable "${DOMAIN}/${LABEL}"

sleep 1
echo
echo "==> status"
launchctl print "${DOMAIN}/${LABEL}" | grep -E "state|pid|last exit" | head -6 || true
echo
echo "✓ panel-backend up. logs: ${PANEL_DIR}/data/panel.log"
echo "  uninstall: launchctl bootout ${DOMAIN} ${PLIST_DST}"
