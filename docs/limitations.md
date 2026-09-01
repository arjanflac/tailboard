# Known Limitations

See also: [Architecture](architecture.md), [Security & Privacy](security.md),
[Platform Support](platform-support.md), and [Roadmap](roadmap.md).

## Behavior and consistency

- Sync is last-write-wins. Concurrent copies are not merged.
- macOS polls the cheap pasteboard change counter because AppKit provides no
  global clipboard-change notification. Windows and supported Wayland
  compositors use event-driven notifications; X11 uses a polling fallback.
- Reconnect recovery sends the latest missed state, not every intermediate
  clipboard change.
- OS-native conversion can change HTML or image representation during a
  round-trip.

## Content

- Individual clipboard payloads are capped at 10 MiB.
- Plain text, HTML, and PNG are the portable formats.
- Platform coercion can reduce rich content to plain text.
- Photos and ordinary files are outside Tailboard; use the native Tailscale
  share target.

## Platforms

- Linux requires `wl-copy`/`wl-paste` or `xclip`.
- iOS does not permit an always-on clipboard watcher. Tailboard reads the iPhone
  clipboard only after a foreground app, keyboard, share, Shortcut, widget, or
  Control Center action.
- TestFlight and public Mac distribution still require release signing and
  external Apple configuration.
- Linux and Windows use the Go desktop tools; Tailboard does not currently ship
  native GUI wrappers for them.

## Security and operations

- The hub can read clipboard contents and stores bounded plaintext history.
- Privacy filters are local, opt-in, and best-effort.
- There is no per-device selective sync or application-layer history
  encryption.
- Development mode is not a hardened network deployment.
- Auto-discovery depends on Tailscale metadata; an explicit hub URL may be
  needed when discovery fails.
- `tg-clip clear` cannot erase content already copied into another device or
  an unrelated clipboard manager.
