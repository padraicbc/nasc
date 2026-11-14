#!/usr/bin/env bash
# Builds the signed Play Store bundle: android/app/build/outputs/bundle/release/app-release.aab
# Needs android/keystore.properties (see RELEASE.md). Refuses to produce a debug-signed bundle.
set -euo pipefail
root=$(cd "$(dirname "$0")/.." && pwd)

if [ ! -f "$root/android/keystore.properties" ]; then
  echo "android/keystore.properties not found. Create your upload key first: see RELEASE.md" >&2
  exit 1
fi

make -C "$root" web
"$root/scripts/build-android.sh" --no-apk   # gomobile AAR only; Gradle below does the signed build
cd "$root/android"
./gradlew clean bundleRelease

aab=app/build/outputs/bundle/release/app-release.aab
if jarsigner -verify -certs "$aab" 2>/dev/null | grep -q "CN=Android Debug"; then
  echo "ERROR: bundle is signed with the debug key. Check keystore.properties." >&2
  exit 1
fi
jarsigner -verify "$aab" >/dev/null || { echo "ERROR: bundle signature does not verify" >&2; exit 1; }

echo
echo "Release bundle: android/$aab ($(du -h "$aab" | cut -f1))"
grep -E "versionCode|versionName" app/build.gradle.kts | sed 's/^ */  /'
echo "Upload it in Play Console > Production (or a testing track)."
