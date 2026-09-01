# Known limitations

- Android 10+ prevents ordinary background apps from reading the clipboard.
  Pixel-to-Mac sends therefore require a visible user action.
- iOS suspends general-purpose apps and restricts pasteboard access. Tailboard
  iOS cannot be an always-on clipboard daemon.
- Apple Universal Clipboard is an Apple continuity feature, not a public
  protocol Tailboard can extend to Android.
- Clipboard ordering is last-write-wins and reconnect recovery carries the
  latest state, not every intermediate copy.
- The Mac relay stores one plaintext current value. Clearing it does not erase
  copies already retained by a phone or a system clipboard manager.
- Each phone's recent-clips list is local to that phone and automatically drops
  entries after 24 hours or beyond the newest 20.
- Tailboard does not transfer files or photos. Use Taildrop.
- macOS 27 is a developer beta; Apple can still change service-management
  behavior before release.
