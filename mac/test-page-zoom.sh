#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")"
test_dir=$(mktemp -d "${TMPDIR:-/tmp}/panel-page-zoom.XXXXXX")
trap 'rm -rf "$test_dir"' EXIT
xcrun swiftc Panel/State/PageZoom.swift tests/PageZoomTests.swift \
  -o "$test_dir/page-zoom-tests"
"$test_dir/page-zoom-tests"
