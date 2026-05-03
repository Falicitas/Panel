#!/usr/bin/env bash
# 一键重 build + 重启 panel。dev 期间用,跑 ~/panel/mac/rebuild.sh 即可。
# 关键:必须带 -destination 'platform=macOS',否则 xcodebuild 会试 build
# x86_64 + arm64 universal,而 NetworkImage 等依赖只 install 了 arm64,会报
# "no such module 'NetworkImage'"。
set -euo pipefail
cd "$(dirname "$0")"
xcodebuild \
  -project Panel.xcodeproj \
  -scheme Panel \
  -configuration Debug \
  -destination 'platform=macOS' \
  -derivedDataPath build \
  build
pkill -f "Panel.app/Contents/MacOS/Panel" 2>/dev/null || true
sleep 1
open build/Build/Products/Debug/Panel.app
