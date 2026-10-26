#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")"
workspace_resources=${1:-../Panel/Resources/dual-notes}
workspace_test_dir=$(mktemp -d "${TMPDIR:-/tmp}/panel-workspace-test.XXXXXX")
trap 'rm -rf "$workspace_test_dir"' EXIT
cp "$workspace_resources/index.html" "$workspace_resources/bundle.js" \
  "$workspace_resources/bundle.css" "$workspace_test_dir/"
xcrun swiftc tests/WorkspaceWebView.swift \
  -module-cache-path "$workspace_test_dir/module-cache" \
  -o "$workspace_test_dir/workspace-webview"
"$workspace_test_dir/workspace-webview" "$workspace_test_dir/index.html" \
  "$PWD/tests/short-term-workspace.js"
