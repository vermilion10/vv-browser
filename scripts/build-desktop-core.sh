#!/usr/bin/env sh
# Builds the vvcore command as the desktop app's sidecar binary, named with
# the host target triple as Tauri expects.
set -eu

root=$(cd "$(dirname "$0")/.." && pwd)
triple=$(rustc -vV | sed -n 's/^host: //p')
ext=""
case "$triple" in
    *windows*) ext=".exe" ;;
esac

out="$root/desktop/src-tauri/binaries/vvcore-$triple$ext"
mkdir -p "$(dirname "$out")"
cd "$root/core"
go build -trimpath -ldflags "-s -w" -o "$out" ./cmd/vvcore
echo "built ${out#"$root"/}"
