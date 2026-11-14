#!/usr/bin/env bash
# Builds the Go core (with the embedded web app) into android/app/libs/nasc.aar via gomobile,
# then assembles the debug APK. Run `make web` first (the Makefile's `android` target does).
set -euo pipefail
root=$(cd "$(dirname "$0")/.." && pwd)

: "${ANDROID_HOME:?Set ANDROID_HOME to your Android SDK, e.g. ~/Android/Sdk}"
# NDK r28+ is needed for 16 KB page-size support (required by Google Play).
ndk_version=${NASC_NDK_VERSION:-28.2.13676358}
export ANDROID_NDK_HOME="$ANDROID_HOME/ndk/$ndk_version"
if [ ! -d "$ANDROID_NDK_HOME/toolchains/llvm" ]; then
  echo "Install Android NDK $ndk_version (Android Studio > SDK Manager > Show Package Details)." >&2
  exit 1
fi

export PATH="$(go env GOPATH)/bin:$PATH"
command -v gomobile >/dev/null || go install golang.org/x/mobile/cmd/gomobile@latest
gomobile init

mkdir -p "$root/android/app/libs"
cd "$root/core"
# arm64 = phones, amd64 = the emulator on a PC.
gomobile bind \
  -target=android/arm64,android/amd64 \
  -androidapi 26 \
  -javapkg "" \
  -ldflags="-s -w -extldflags=-Wl,-z,max-page-size=16384,-z,common-page-size=16384" \
  -o "$root/android/app/libs/nasc.aar" \
  .

if [ "${1:-}" = "--no-apk" ]; then
  echo "Built android/app/libs/nasc.aar"
  exit 0
fi

cd "$root/android"
./gradlew assembleDebug
echo
echo "Built android/app/build/outputs/apk/debug/app-debug.apk"
echo "Install:  adb install -r android/app/build/outputs/apk/debug/app-debug.apk"
