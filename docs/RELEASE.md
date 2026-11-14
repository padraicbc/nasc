# Releasing Nasc to Google Play

## One-time setup

1. **Decide the application ID now.** It is `app.nasc` and can never change once an app is
   published under it. If you own a domain, prefer it (e.g. `com.yourdomain.nasc`): change
   `applicationId` in `android/app/build.gradle.kts` (and `namespace` if you also want the
   Kotlin package to match). Check the name "Nasc" is free on Play and not someone's trademark.
2. **Create the upload key** (keep the file and passwords somewhere safe and backed up):
   ```bash
   keytool -genkeypair -v -keystore ~/nasc-upload.jks -alias upload \
     -keyalg RSA -keysize 4096 -validity 10000
   ```
3. Create `android/keystore.properties` (gitignored):
   ```properties
   storeFile=/home/you/nasc-upload.jks
   storePassword=...
   keyAlias=upload
   keyPassword=...
   ```
4. In Play Console, create the app and enrol in **Play App Signing** (the default). Google holds the
   real signing key; yours is only the upload key, which can be reset if lost.

## Every release

1. Bump `versionCode` (must increase every upload) and `versionName` in `android/app/build.gradle.kts`.
2. `make test`
3. `./scripts/release.sh` → `android/app/build/outputs/bundle/release/app-release.aab`
4. Upload to a testing track first, install from Play on a real phone, then promote.

Already handled in the build: target API 36, R8 shrinking with keep rules for gomobile and the
JavaScript bridge, 16 KB page-size alignment (checked: zip and ELF), no debuggable flag,
`allowBackup=false`, cleartext traffic limited to the app's own loopback server, native debug
symbols included, and a guard that refuses a debug-signed bundle.

## Play Console items you must do (I can't do these for you)

| Item | Notes |
|---|---|
| **Foreground service declaration** | App content → Foreground service. Type: *Special use* (keeps interactive SSH sessions connected in the background). Needs a short **demo video**: open a session, press Home, show the "Nasc is running" notification, return and the session is still alive. |
| **Privacy policy URL** | Publish `docs/PRIVACY.md` at a public URL (e.g. on your own domain) and paste it into the Play Console and the listing. Needs to load without login and stay up. |
| **Data safety form** | Nothing is collected or shared by the developer (see PRIVACY.md). Review the wording against the current form. |
| **Content rating** | Questionnaire; expect Everyone. |
| **Target audience** | 18+ / not for children. |
| **App access** | Tell reviewers the app needs an SSH server. Consider giving a test host + credentials, or explain how to reach one. Reviewers who can't log in anywhere may reject. |
| **Encryption** | The app implements SSH. Answer export-compliance questions honestly; standard SSH/AES via open-source libraries is typically mass-market self-classification. Verify for your jurisdiction. |
| **Testing requirement** | Personal developer accounts created after Nov 2023 must run a closed test (12+ testers, 14 days) before requesting production access. Check your account type. |
| **Store listing** | Text is in `docs/STORE_LISTING.md`; graphics are in `docs/store/` (see below). Still needed: contact email and privacy policy URL. |

## Store graphics (`docs/store/`)

| File | Play Console field |
|---|---|
| `icon-512.png` | App icon (512×512) |
| `feature-graphic-1024x500.png` | Feature graphic |
| `phone-screenshots/01…05-*.png` | Phone screenshots (1080×2400; upload in order) |

`icon.svg` and `feature-graphic.svg` are the editable sources. Screenshots were rendered from the real web UI at phone size against a scripted demo SSH server (host `demo@…`, no real servers or credentials shown). Retake them if the UI changes. 7-inch/10-inch tablet screenshots are optional.

## Open-source notices

Ship attribution for: Go + `golang.org/x/crypto` (BSD-3), `github.com/coder/websocket` (ISC),
Svelte/SvelteKit (MIT), xterm.js (MIT), AndroidX (Apache-2.0). An in-app "Licenses" screen or a
link in the store listing both work.

## GitHub releases (sideloadable APK)

1. Create a separate GitHub key (`keytool -genkeypair -v -keystore ~/nasc-github.jks -alias nasc -keyalg RSA -keysize 4096 -validity 36500`)
   and `android/keystore-github.properties` (same fields as `keystore.properties`, pointing at that file).
   Don't reuse the Play upload key. Keep this one forever and back it up: users can only update in place if later APKs have the same key. Play re-signs the
   Play build with its own key, so the two channels aren't interchangeable (uninstall to switch).
2. Bump `versionCode`/`versionName`, update `docs/RELEASE_NOTES.md`, `make test`.
3. `./scripts/release-apk.sh` → `dist/nasc-<version>.apk` and `dist/SHA256SUMS`.
4. `git tag v<version> && git push --tags`, then run the `gh release create …` command the script prints
   (needs `gh auth login`).
