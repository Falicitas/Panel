#!/usr/bin/env bash
# 从当前工作区构建并安装双栏服务，可独立于 panel-backend 运行。
set -euo pipefail

SOURCE_DIR="$(cd "$(dirname "$0")/.." && pwd)"
PANEL_DIR="${HOME}/panel"
BIN_DIR="${PANEL_DIR}/bin"
BIN="${BIN_DIR}/panel-dual-notes"
LABEL="com.kinesis.panel.dual-notes"
PLIST_NAME="${LABEL}.plist"
PLIST_DST="${HOME}/Library/LaunchAgents/${PLIST_NAME}"
DOMAIN="gui/$(id -u)"

mkdir -p "${BIN_DIR}" "${PANEL_DIR}/data" "${HOME}/Library/LaunchAgents"
STAGING_DIR="$(mktemp -d "${BIN_DIR}/.dual-notes-install.XXXXXX")"
trap 'rm -rf "${STAGING_DIR}"' EXIT

# 构建和 plist 检查成功后再停止旧的双栏服务。
echo "==> building panel-dual-notes"
( cd "${SOURCE_DIR}/backend" && go build -o "${STAGING_DIR}/panel-dual-notes" ./cmd/panel-dual-notes )
sed -e "s|__PANEL_BIN__|${BIN}|g" \
    -e "s|__PANEL_DIR__|${PANEL_DIR}|g" \
    "${SOURCE_DIR}/scripts/${PLIST_NAME}" > "${STAGING_DIR}/${PLIST_NAME}"
plutil -lint "${STAGING_DIR}/${PLIST_NAME}"

if launchctl print "${DOMAIN}/${LABEL}" >/dev/null 2>&1; then
    launchctl bootout "${DOMAIN}/${LABEL}"
fi

# 只检查双栏端口，其他后端的启停状态不影响安装。
for attempt in {1..20}; do
    if ! lsof -nP -iTCP:8090 -sTCP:LISTEN >/dev/null 2>&1; then
        break
    fi
    sleep 0.25
done
if lsof -nP -iTCP:8090 -sTCP:LISTEN >/dev/null 2>&1; then
    echo "127.0.0.1:8090 仍被占用，请停止占用进程后重试。" >&2
    lsof -nP -iTCP:8090 -sTCP:LISTEN >&2
    exit 1
fi

mv "${STAGING_DIR}/panel-dual-notes" "${BIN}"
mv "${STAGING_DIR}/${PLIST_NAME}" "${PLIST_DST}"
launchctl enable "${DOMAIN}/${LABEL}"
launchctl bootstrap "${DOMAIN}" "${PLIST_DST}"

for attempt in {1..40}; do
    if curl --fail --silent --max-time 1 \
        http://127.0.0.1:8090/api/health >/dev/null; then
        echo "✓ 双栏服务已启动：http://127.0.0.1:8090"
        echo "  日志：${PANEL_DIR}/data/dual-notes.log"
        exit 0
    fi
    sleep 0.25
done
echo "双栏服务未通过健康检查，请检查 ${PANEL_DIR}/data/dual-notes.log。" >&2
launchctl print "${DOMAIN}/${LABEL}" >&2 || true
exit 1
