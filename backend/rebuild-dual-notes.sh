#!/usr/bin/env bash
# 仅构建并重启双栏服务，首次运行也会安装独立 launchd 启动项。
set -euo pipefail
exec "$(cd "$(dirname "$0")/.." && pwd)/scripts/install-dual-notes.sh"
