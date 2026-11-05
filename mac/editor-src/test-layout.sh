#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")"
# Optionally point at another checkout's built resources to verify the regression.
layout_resources=${1:-../Panel/Resources/dual-notes}
layout_test_dir=$(mktemp -d "${TMPDIR:-/tmp}/panel-layout-test.XXXXXX")
trap 'rm -rf "$layout_test_dir"' EXIT
cp "$layout_resources/index.html" "$layout_resources/bundle.js" "$layout_resources/bundle.css" "$layout_test_dir/"
xcrun swiftc tests/ResizeWebView.swift -module-cache-path "$layout_test_dir/module-cache" -o "$layout_test_dir/layout-webview"
for layout_script in dual-notes-layout.js focus-scroll.js find-scroll.js; do
  "$layout_test_dir/layout-webview" "$layout_test_dir/index.html" "$PWD/tests/$layout_script"
done
