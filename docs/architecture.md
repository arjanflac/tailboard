# Architecture

```text
Mac NSPasteboard <-> Tailboard Engine + embedded hub <-> Pixel foreground service
                                      |
                                      +-- optional foreground iPhone client

Mac <---------------- Apple Universal Clipboard ----------------> iPhone

Mac / Pixel / iPhone <--------------- Taildrop -----------------> files
```

## Mac lifecycle

`Tailboard.app` contains the signed `Tailboard Engine.app` login item. When the
outer app opens, it registers or updates that item with `SMAppService` and
exits. Only the nested engine remains running.

The engine reads `NSPasteboard.changeCount` every 100 ms through a direct AppKit
bridge. It only reads the pasteboard's native string representation when the
count changes and posts that plain text to its embedded hub. This avoids both
literal HTML payloads and the old `osascript` subprocess on every poll.
Pasteboard entries marked with macOS's `org.nspasteboard.ConcealedType` or
`org.nspasteboard.TransientType` conventions are ignored. This keeps temporary
paste-helper payloads (including Wispr Flow's paste-and-restore sequence) and
password-manager secrets out of Tailboard without delaying ordinary copies.

The embedded hub listens only on the Mac's Tailscale address, stores bounded
history in SQLite, assigns monotonic sequences, and broadcasts updates over
WebSocket. The old loopback desktop control server has been removed with the
menu UI. Its protocol and schema contain text only; images and generic blobs are
not accepted or stored.

## Android lifecycle

The Pixel foreground service keeps a WebSocket connected and applies inbound
text as soon as it arrives. Android does not permit a normal background app to
read copied text, so outbound text is explicitly triggered by the Quick
Settings tile, an app action, or the text share target.

## iOS lifecycle

The iOS app is a foreground-only optional client with no extensions or
background UI. It is not responsible for normal Mac/iPhone continuity; Apple
Universal Clipboard already handles that for devices on the same Apple Account.
Tailboard iOS is only useful when a direct Pixel/iPhone bridge is wanted.
