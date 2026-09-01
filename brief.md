# Tailboard

Tailboard is a private clipboard bridge for macOS, Android, and iPhone devices
on the same Tailscale tailnet. It is derived from the MIT-licensed
`thalysguimaraes/tg-clipboard` project.

## Intended setup

- One Mac runs the embedded hub and clipboard engine from inside
  `Tailboard.app`.
- Android receives clipboard updates through a foreground service. Its Quick
  Settings tile and text share target send the current text to the other
  devices.
- iPhone exposes the current clip, history, keyboard, text/link share target,
  Shortcuts, widget, and Control Center controls. Clipboard reads are
  user-initiated because iOS suspends general-purpose background work.
- The Mac menu-bar app reports sync health, devices, the current clip, and a
  pause switch over the engine's loopback-only control API.

The devices do not need to share an Apple ID. Tailboard uses the existing
Tailscale connection and its own small clipboard broker rather than Universal
Clipboard or iCloud.

Photos and files are outside Tailboard's scope. Use the native Tailscale share
target on each platform.

## Local configuration

Copy `config.example.env` to ignored `config.local.env` and fill in the local
device names. Never commit that local file.

## Privacy boundary

Clipboard history and mobile caches are stored locally and rely on OS disk
encryption. Clearing the hub does not retroactively erase content already
written into another device's clipboard or an unrelated clipboard manager.

## Publishing

The code lives in a standalone private repository with upstream Git history and
MIT attribution intact. Do not publish binaries until the blocking items in
`docs/public-release-checklist.md` are resolved.
