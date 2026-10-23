#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")"
math_test_dir=$(mktemp -d "${TMPDIR:-/tmp}/panel-math-test.XXXXXX")
trap 'rm -rf "$math_test_dir"' EXIT
./node_modules/.bin/esbuild src/dual-notes-math.browser-test.ts \
  --bundle --format=iife --target=es2020 \
  --loader:.woff=dataurl --loader:.woff2=dataurl --loader:.ttf=dataurl \
  --outfile="$math_test_dir/test.js" >/dev/null
printf '%s\n' '<!doctype html><meta charset="utf-8"><link rel="stylesheet" href="test.css"><body><script src="test.js"></script>' \
  > "$math_test_dir/index.html"
xcrun swiftc tests/MathWebView.swift -o "$math_test_dir/math-webview"
"$math_test_dir/math-webview" "$math_test_dir/index.html"
