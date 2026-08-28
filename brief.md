# Tailboard

Tailboard is a private clipboard and file bridge for a Mac, Android phone, and
iPhone connected to the same Tailscale tailnet. It is based on the
MIT-licensed `thalysguimaraes/tg-clipboard` project and extends it with mobile
file transfers, a local desktop control surface, and personal platform apps.

## Intended setup

- One Mac carries the embedded hub and clipboard agent as a login LaunchAgent.
- Android receives desktop clipboard updates automatically. Its Quick Settings
  tile sends copied text, and its single share target sends text, photos, or
  files to the configured default device.
- Android automatically accepts verified file offers from registered macOS or
  iOS devices and publishes them under `Downloads/Tailboard`.
- The iOS share extension sends files to a configurable default device. iOS
  clipboard reads remain user initiated because iOS suspends general-purpose
  background work. Incoming Mac files are accepted automatically while the app
  is foregrounded and saved in the app's Files container.
- The Mac exposes a loopback-only web control surface and an optional lean
  menu-bar app. Both reuse the same engine. A bundled macOS Share extension
  makes Tailboard available in Finder's right-click Share menu.

The iPhone and Mac do not need to share an Apple ID. Tailboard uses Tailscale
and the local hub rather than Universal Clipboard or iCloud.

## Local configuration

Copy `config.example.env` to ignored `config.local.env` and fill in the local
tailnet names and auto-accept device IDs. Never commit that local file.

## Privacy boundary

Clipboard history and mobile caches are stored locally and rely on OS disk
encryption. Tailboard includes explicit cleanup paths, but clearing the hub
does not erase an IME's private clipboard history or unrelated photo libraries.

## Publishing

The code now lives in a standalone private repository with upstream Git history
and attribution intact. Do not switch it public or publish binaries until the
blocking items in `docs/public-release-checklist.md` are resolved, especially
the naming/trademark and public authentication decisions.
