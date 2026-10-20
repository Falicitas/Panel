#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")"
test_dir=$(mktemp -d "${TMPDIR:-/tmp}/panel-dual-notes-client.XXXXXX")
trap 'rm -rf "$test_dir"' EXIT
xcrun swiftc -swift-version 5 -target "$(uname -m)-apple-macosx14.0" \
  Panel/Net/APIClient.swift Panel/Net/BackendClient.swift \
  Panel/Models/*.swift Panel/Voice/AudioPlayer.swift \
  tests/DualNotesClientTests.swift \
  -o "$test_dir/dual-notes-client-tests"
"$test_dir/dual-notes-client-tests"
