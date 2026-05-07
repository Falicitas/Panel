#!/usr/bin/env bash
# 一键把 mac/design/Icon-3d.svg 部署成 Panel.app icon。dev 期间调 SVG (深度 / 位置 /
# 颜色) 后跑 ~/panel/mac/deploy-icon.sh 即可,不要再手动拼 sips chain。
#
# 流程:
#   1. rsvg-convert 把 SVG 渲成 1024 master PNG (mac/design/Icon-3d-1024.png)
#      关键: 用 rsvg-convert 不要用 qlmanage / sips, 后两者会把 alpha 通道填白
#      (squircle 圆角外的透明区会变成白边, 在 macOS 14+ 又被强加 squircle mask 显
#      示成"圆角白边"). rsvg-convert 保 alpha.
#   2. sips 缩 10 个 size 替进 Panel/Assets.xcassets/AppIcon.appiconset/
#      (16/32/128/256/512 + @2x). actool build 时会自动从 Asset Catalog 编进 .icns.
#   3. 调 rebuild.sh 复用 xcodebuild + pkill + open.
#
# 依赖:
#   - rsvg-convert: brew install librsvg
#   - sips: macOS 内置
#   - xcodebuild: Xcode CLT
#
# Dock 没刷新到新 icon? macOS 会把 icon 缓存到 LaunchServices/IconServices, 重 build
# 不一定吹掉. 跑下面这条 (要 sudo, 没集成进脚本因为不该静默 sudo):
#   sudo rm -rfv /Library/Caches/com.apple.iconservices.store && \
#     sudo find /private/var/folders/ \
#       \( -name com.apple.dock.iconcache -or -name com.apple.iconservices \) \
#       -exec rm -rfv {} \; ; \
#     sleep 3 ; killall Dock Finder
#
# 历史: issue-30 (3D 化版定稿), Icon-3d.svg 是 design 源, 跟 Panel/Assets 里的 PNG 跟
# .icns 都是它派生的; 改 icon 永远改 SVG, 不要直接改 PNG.

set -euo pipefail
cd "$(dirname "$0")"

SVG="design/Icon-3d.svg"
MASTER="design/Icon-3d-1024.png"
ICONSET="Panel/Assets.xcassets/AppIcon.appiconset"

command -v rsvg-convert >/dev/null \
  || { echo "rsvg-convert 未装: brew install librsvg" >&2; exit 1; }
[ -f "$SVG" ] || { echo "SVG 源不存在: $PWD/$SVG" >&2; exit 1; }

echo "[1/3] rsvg-convert SVG → 1024 master PNG"
rsvg-convert -w 1024 -h 1024 "$SVG" -o "$MASTER"

echo "[2/3] sips 缩 10 个 size 进 Asset Catalog"
sips -z 16 16     "$MASTER" --out "$ICONSET/icon_16x16.png"      >/dev/null
sips -z 32 32     "$MASTER" --out "$ICONSET/icon_16x16@2x.png"   >/dev/null
sips -z 32 32     "$MASTER" --out "$ICONSET/icon_32x32.png"      >/dev/null
sips -z 64 64     "$MASTER" --out "$ICONSET/icon_32x32@2x.png"   >/dev/null
sips -z 128 128   "$MASTER" --out "$ICONSET/icon_128x128.png"    >/dev/null
sips -z 256 256   "$MASTER" --out "$ICONSET/icon_128x128@2x.png" >/dev/null
sips -z 256 256   "$MASTER" --out "$ICONSET/icon_256x256.png"    >/dev/null
sips -z 512 512   "$MASTER" --out "$ICONSET/icon_256x256@2x.png" >/dev/null
sips -z 512 512   "$MASTER" --out "$ICONSET/icon_512x512.png"    >/dev/null
cp "$MASTER"      "$ICONSET/icon_512x512@2x.png"

echo "[3/3] rebuild.sh (xcodebuild + pkill + open)"
./rebuild.sh

echo
echo "✅ 部署完成. Dock 没刷新到新 icon? 看本脚本顶部注释里的 cache nuke 命令."
