---
title: Nasc Privacy Policy
permalink: /privacy/
---

# Nasc Privacy Policy

_Last updated: 2026-11-12. Replace the contact details before publishing._

Nasc is an SSH client. It is designed so that your data stays on your device.

## What we collect

Nothing. Nasc has no accounts, no analytics, no advertising, no crash reporting service and no
servers of its own. The developer receives no data from the app.

## What the app stores on your device

Everything below is kept in the app's private storage and is not sent anywhere by the app:

- **Saved hosts** (name, address, port, username).
- **SSH private keys** you import or generate. They are encrypted with AES-256-GCM. The encryption
  key is protected by the Android Keystore (hardware-backed where your device supports it) and, if
  you turn on App lock, can only be released after your fingerprint, face or screen lock.
  Passwords and key passphrases are not saved.
- **Trusted server fingerprints** (known hosts).
- **Command history** per host, used for autocomplete. This is stored on the device as plain text
  in the app's private storage. Commands starting with a space, and anything typed at a password
  prompt, are not recorded. You can clear a host's history from its edit screen.

Uninstalling the app deletes all of this. App data is excluded from Android backups.

## Network use

Nasc connects only to the SSH servers you choose, using the credentials you provide. What you do
on those servers is between you and them. The app does not contact any other service.

## Permissions

- **Internet** – to connect to your servers.
- **Notifications and foreground service** – to show that sessions are active and keep them
  connected while the app is in the background.
- **Biometrics/screen lock** (optional) – only for App lock.

## Children

Nasc is not directed at children.

## Changes and contact

If this policy changes, the date above will change. Contact: YOUR-EMAIL-HERE
