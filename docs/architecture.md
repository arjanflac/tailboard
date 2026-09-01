# Architecture

See also: [Security & Privacy](security.md), [Known Limitations](limitations.md),
[Platform Support](platform-support.md), and [README](../README.md).

Tailboard is a clipboard-only system with one small broker and platform-native
clients.

```text
Mac clipboard <-> Tailboard Engine <-> embedded hub <-> Android app
                                      |
                                      +-------------> iPhone app/extensions
```

All cross-device traffic uses addresses reachable through the user's existing
Tailscale connection.

## Components

| Component | Responsibility |
| --- | --- |
| `tg-clipboard` | Standalone HTTP/WebSocket clipboard broker for users who want an independently managed hub. |
| `tg-clipd` / Tailboard Engine | Watches a desktop clipboard, applies remote clips, and can carry the embedded broker role. |
| `tg-clip` | Scriptable clipboard, history, device, status, pause, and clear commands. |
| macOS app | Menu-bar status and controls; embeds and registers Tailboard Engine with `SMAppService`. |
| Android app | Foreground WebSocket client, clipboard UI, text share target, and Quick Settings action. |
| iOS app and extensions | Foreground clipboard UI, keyboard, text/link share target, widget, Shortcuts, and Control Center controls. |

## Clipboard flow

1. A client reads a local clipboard item after an OS event or user action.
2. The client posts the item with its MIME type, source, and stable device ID.
3. The hub assigns a monotonic sequence, deduplicates identical content, stores
   bounded history in SQLite, and publishes the latest item over WebSocket.
4. Connected clients apply the item locally and ignore their own echo.
5. Reconnecting clients request the latest sequence they missed.

Clipboard state is last-write-wins. The hub supports plain text, HTML, and PNG
with a 10 MiB item limit.

## Embedded hub

The normal personal deployment runs the broker inside Tailboard Engine on the
Mac. It listens on the Mac's Tailscale address and persists state under the
engine's application-support directory. This removes the need for a separate
always-on server while keeping a single source of ordering and history.

The standalone `tg-clipboard` binary remains useful for Linux, Windows, or a
dedicated host. It can join a tailnet through `tsnet`.

## Local control API

The Mac app talks to Tailboard Engine through a loopback-only HTTP endpoint.
That API exposes sync state, the device roster, current clip metadata, and the
pause switch. It is not reachable from the tailnet.

## Persistence and retention

- SQLite stores bounded clipboard history and device metadata.
- Raw clipboard data is retained for up to 24 hours by default.
- In-memory and persistent byte budgets prevent large clipboard images from
  growing the always-on process without bound.
- Stale offline device records expire after 30 days.

Photos and ordinary files never enter this data plane. They are handled by the
native Tailscale application.
