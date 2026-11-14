#!/usr/bin/env bash
# Builds a signed, installable APK for GitHub Releases: dist/nasc-<version>.apk + SHA256SUMS.
# Signs with the GitHub key from android/keystore-github.properties (separate from the Play upload
# key used by release.sh; see docs/RELEASE.md).
#
# NOTE: an APK signed with your own key can't be updated in place by the Play Store build (Play
# re-signs with its own key), and vice versa. Use a separate key for GitHub if you want both
# channels, or tell users to uninstall before switching.
set -euo pipefail
root=$(cd "$(dirname "$0")/.." && pwd)

if [ ! -f "$root/android/keystore-github.properties" ]; then
  echo "android/keystore-github.properties not found. Create the GitHub signing key first: see docs/RELEASE.md" >&2
  exit 1
fi
: "${ANDROID_HOME:?Set ANDROID_HOME to your Android SDK}"
apksigner=$(ls -d "$ANDROID_HOME"/build-tools/*/apksigner | sort -V | tail -1)

make -C "$root" web
"$root/scripts/build-android.sh" --no-apk
cd "$root/android"
./gradlew clean assembleRelease -Pnasc.keystore=keystore-github.properties

apk=app/build/outputs/apk/release/app-release.apk
[ -f "$apk" ] || { echo "ERROR: $apk missing (is the build unsigned?)" >&2; exit 1; }
"$apksigner" verify --print-certs "$apk" | tee /tmp/nasc-certs.txt
if grep -q "CN=Android Debug" /tmp/nasc-certs.txt; then
  echo "ERROR: APK is signed with the debug key. Check keystore-github.properties." >&2; exit 1
fi

version=$(grep -oP 'versionName = "\K[^"]+' app/build.gradle.kts)
mkdir -p "$root/dist"
out="$root/dist/nasc-$version.apk"
cp "$apk" "$out"
(cd "$root/dist" && sha256sum "nasc-$version.apk" > SHA256SUMS)

echo
echo "Built $out ($(du -h "$out" | cut -f1))"
cat "$root/dist/SHA256SUMS"
echo
echo "Publish:  gh release create v$version dist/nasc-$version.apk dist/SHA256SUMS \\"
echo "            --title \"Nasc $version\" --notes-file docs/RELEASE_NOTES.md"
