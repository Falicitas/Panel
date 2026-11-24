#!/usr/bin/env bash
# 一键重 build + 重启 panel。dev 期间用,跑 ~/panel/mac/rebuild.sh 即可。
# 关键:必须带 -destination 'platform=macOS',否则 xcodebuild 会试 build
# x86_64 + arm64 universal,而 NetworkImage 等依赖只 install 了 arm64,会报
# "no such module 'NetworkImage'"。
set -euo pipefail
cd "$(dirname "$0")"

# 签名:pbxproj 写死 Developer ID (9MA3NPTKD4),私钥只在开发者本机钥匙串。
# 有证书就用,签名 hash 稳定,TCC 授权 (输入监控/辅助功能) 只问一次;没有
# (换新电脑) 就回退 ad-hoc,否则 xcodebuild 直接报找不到证书。ad-hoc 每次
# build 的 CDHash 都变,macOS 当新 app,可能要重新授权。
SIGN_IDENTITY="Developer ID Application: Tingzhang Cai (9MA3NPTKD4)"
SIGN_FLAGS=""
if security find-identity -v -p codesigning 2>/dev/null | grep -qF "$SIGN_IDENTITY"; then
  echo "==> 签名: $SIGN_IDENTITY"
else
  echo "==> 本机没有 $SIGN_IDENTITY,回退 ad-hoc 签名 (每次 rebuild 后可能需重新授权)"
  SIGN_FLAGS='CODE_SIGN_IDENTITY=- DEVELOPMENT_TEAM='
fi

# SIGN_FLAGS 故意不加引号:空串展开成无参数,非空时拆成两个 xcodebuild 覆盖项。
xcodebuild \
  -project Panel.xcodeproj \
  -scheme Panel \
  -configuration Debug \
  -destination 'platform=macOS' \
  -derivedDataPath build \
  build ${SIGN_FLAGS}
pkill -f "Panel.app/Contents/MacOS/Panel" 2>/dev/null || true
sleep 1
open build/Build/Products/Debug/Panel.app
