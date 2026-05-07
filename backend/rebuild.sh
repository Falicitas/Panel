#!/usr/bin/env bash
# 从当前 worktree build + trigger launchd respawn (label com.kinesis.panel)。
# 用户在 worktree 根目录跑 ./backend/rebuild.sh 即可, 不必 cd。
set -euo pipefail
cd "$(dirname "$0")"
go build -o ~/panel/bin/panel-backend ./
kill -TERM "$(pgrep -f 'panel/bin/panel-backend')" 2>/dev/null || true
