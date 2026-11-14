#!/usr/bin/env bash
# Installs the debug APK on one device. adb refuses to guess when several are attached, so:
#   - ANDROID_SERIAL=<serial> make android   picks one explicitly
#   - otherwise a single physical phone wins over emulators
set -euo pipefail
apk="$(cd "$(dirname "$0")/.." && pwd)/android/app/build/outputs/apk/debug/app-debug.apk"

if [ -z "${ANDROID_SERIAL:-}" ]; then
  mapfile -t all < <(adb devices | awk 'NR>1 && $2=="device" {print $1}')
  mapfile -t phones < <(printf '%s\n' "${all[@]:-}" | grep -v '^emulator-' || true)
  if [ "${#all[@]}" -eq 0 ]; then
    echo "No device attached (adb devices is empty); APK is at $apk" >&2
    exit 1
  elif [ "${#all[@]}" -eq 1 ]; then
    export ANDROID_SERIAL="${all[0]}"
  elif [ "${#phones[@]}" -eq 1 ]; then
    export ANDROID_SERIAL="${phones[0]}"
    echo "Several devices attached; using the physical one: $ANDROID_SERIAL"
  else
    echo "Several devices attached: ${all[*]}" >&2
    echo "Pick one:  ANDROID_SERIAL=<serial> make android" >&2
    exit 1
  fi
fi
echo "Installing on $ANDROID_SERIAL"
adb install -r "$apk"
