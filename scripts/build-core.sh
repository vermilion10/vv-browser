#!/usr/bin/env sh
# Builds the Go network core into android/app/libs/vvcore.aar.
# Requires ANDROID_HOME and an installed NDK (ANDROID_NDK_HOME, or the newest
# one under $ANDROID_HOME/ndk).
set -eu

root=$(cd "$(dirname "$0")/.." && pwd)

if [ -z "${ANDROID_NDK_HOME:-}" ]; then
    ANDROID_NDK_HOME=$(ls -d "$ANDROID_HOME"/ndk/*/ | sort -V | tail -n 1)
    export ANDROID_NDK_HOME
fi

mkdir -p "$root/android/app/libs"
cd "$root/core"
go tool gomobile bind \
    -trimpath -ldflags "-s -w" \
    -target=android/arm64,android/amd64 \
    -androidapi 33 \
    -javapkg dev.vvbrowser \
    -o "$root/android/app/libs/vvcore.aar" \
    ./mobile
echo "built android/app/libs/vvcore.aar"
