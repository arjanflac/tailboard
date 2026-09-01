# Embedded Hub Decision

Status: implemented.

Tailboard does not require a separate broker machine. `tg-clipd --embed-hub`
runs the existing clipboard broker inside the desktop agent process.

## Why this design

Clipboard sync needs one authority to assign sequence numbers, retain the latest
item for reconnecting phones, and provide a stable WebSocket endpoint. Embedding
that authority in the Mac keeps the model simple:

- one installed Mac application,
- one lightweight background engine,
- one SQLite database,
- one address reachable through the existing Tailscale client.

This removes a dedicated server without pretending the system is peer-to-peer.
When the Mac is offline, clipboard synchronization pauses.

## Networking

The embedded hub binds to the Mac's normal Tailscale address. It deliberately
does not embed `tsnet`: doing so would create a second tailnet identity and a
second userspace networking stack inside the same application.

The standalone `tg-clipboard` binary still uses `tsnet` when an independent
hub node is desirable.

## Operational properties

- The embedded role is enabled with `--embed-hub`.
- The default port is 9437.
- State lives under the engine's application-support directory.
- The role does not float or elect a replacement automatically.
- Running more than one hub creates separate clipboard histories; clients must
  point at one canonical hub.

For the personal Mac/Pixel/iPhone setup, the Mac is the canonical hub.
