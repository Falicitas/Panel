#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")"
test_dir=$(mktemp -d "${TMPDIR:-/tmp}/panel-autodl-page.XXXXXX")
trap 'rm -rf "$test_dir"' EXIT
xcrun swiftc -swift-version 5 -target "$(uname -m)-apple-macosx14.0" \
  -module-cache-path "$test_dir/module-cache" \
  Panel/Net/AutoDLPage.swift Panel/Views/AutoDLView.swift \
  Panel/ContentView.swift Panel/State/Tab.swift \
  tests/AutoDLPageTests.swift \
  -o "$test_dir/autodl-page-tests"
"$test_dir/autodl-page-tests" Panel/Resources/Info.plist
