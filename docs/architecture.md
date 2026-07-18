# Architecture

See also: [Security & Privacy](security.md), [Transfer E2EE Decision](e2ee-transfers.md), [Known Limitations](limitations.md), [Platform Support](platform-support.md), [Roadmap](roadmap.md), [README](../README.md)

ClipHub is built around a single tailnet broker role with two distinct primitives: broadcast, ephemeral clipboard state and explicit, durable-until-claimed transfers targeted to a registered device. The role may run as standalone `cliphub` or inside a normal desktop agent via `clipd --embed-hub`; a dedicated hub machine is optional.

## System layout

```text
local clipboard <-> clipd ----------------------+
                                                |
tailclip ------------------------------------+  |
                                             |  v
iOS app / share extension / keyboard --> cliphub --> SQLite history + device registry
                                             ^
                                             |
                               WebSocket stream + REST API

sender -- resumable upload --> disk spool --> receiver
```

## Core components

| Component | Responsibility |
| --- | --- |
| `cliphub` | Central broker. Stores clipboard history, device/transfer metadata, and transfer spool files; exposes REST + targeted WebSocket events. |
| `clipd` | Desktop agent. Watches clipboard changes, enforces local privacy policy, applies remote updates, receives targeted transfers, and serves a loopback-only desktop control surface. |
| `tailclip` | Scriptable client for clipboard operations, device discovery, and resumable send/receive workflows. |
| iOS app and extensions | Device-first companion with clipboard/history surfaces, transfer inbox, keyboard, share extension, widgets, App Intents, and background uploads. |

## Data flow

1. A client discovers the hub URL from Tailscale metadata unless `--hub` or `CLIPHUB_HUB` overrides it.
2. `clipd` waits for a native change event where available. Polling backends check a cheap change sequence before reading the richest available content in this order: `image/png`, `text/html`, then `text/plain`.
3. `clipd` can optionally apply local privacy rules before upload. Blocked items stay local and can optionally clear the local clipboard.
4. The client hashes the content and MIME type. If the item is new and not one the client just wrote itself, it sends the item to `cliphub`.
5. `cliphub` stores the clip, assigns a monotonic `seq`, and broadcasts it to WebSocket subscribers.
6. Other clients receive the update, apply it locally, read back what the OS actually stored, and mark that result as self-written so the next poll does not loop the same item back to the hub.
7. Reconnecting clients can resume from `since_seq` to catch up on missed items.

## Transfer flow

1. Clients register stable device IDs and capabilities; online state comes from live WebSocket connections.
2. A sender creates a transfer manifest naming one target device and declaring each file's size, MIME type, and SHA-256.
3. The sender uploads files in resumable chunks. Directories are represented as deterministic per-file manifests.
4. The hub verifies each completed file. Only a fully uploaded transfer becomes `offered`.
5. The target receives a WebSocket offer and accepts, declines, or follows its local allowlist policy.
6. Downloads support byte ranges. The receiver verifies SHA-256, writes through safe temporary paths, then acknowledges completion.
7. Completion, cancellation, or expiry removes spool data. Metadata is not a permanent transfer archive.

For two online desktop endpoints, `tailclip send --direct` starts a bearer-scoped, range-capable file server on the sender's Tailscale IP and places only the offer metadata on the hub. A receiver advertising `direct-fetch` downloads from that endpoint and still verifies SHA-256 before completion. If the target is offline, lacks the capability, the sender cannot bind its tailnet address, or the hub rejects direct metadata, the command transparently uses the durable spool path.

## Desktop companion

`clipd` serves a loopback-only control surface at `127.0.0.1:9438` by default. `clipd --tray` opens it in the default browser. It is a thin client over the running agent and hub APIs:

- device cards are file drop targets,
- active transfers show uploaded progress and state,
- incoming offers expose Accept/Decline and completed downloads expose Show in folder,
- clipboard pause/resume changes the live agent state.

The listener rejects non-loopback configuration and cross-origin browser requests. Set `--control-addr off` for a strictly headless agent.

## Persistence and retention

- The hub persists clipboard history in SQLite.
- Device and transfer metadata are also stored in SQLite; transfer bodies are files under the configured spool directory.
- In the default tailnet mode, the database lives at `~/.config/cliphub/tsnet/clips.db`.
- Retention defaults to a 24 hour TTL and a history depth of 50 items, both configurable on the hub.
- Transfer TTL defaults to 48 hours and is separately bounded by spool quota and per-transfer limits.
- The hub stores both hashes and raw clipboard content because it needs to replay clipboard data, not just detect duplicates.

## Transport modes

### Embedded hub role

- `clipd --embed-hub` serves the full broker API on port 9437 of that machine's Tailscale address and stores broker state beneath the agent state directory.
- Other clients prefer the historic named hub, then discover any peer advertising `hub_role` through `/api/capabilities`.
- The role does not float automatically. When its carrier is offline, sync and asynchronous transfer delivery pause.

### Normal mode

- `cliphub` runs as a `tsnet` node inside your tailnet.
- When Tailscale HTTPS is enabled, the hub serves HTTPS on its MagicDNS hostname.
- If HTTPS is not enabled, the hub falls back to plain HTTP on the tailnet.

### Development mode

- `cliphub -dev` listens on localhost by default and skips the tailnet identity layer.
- It is useful for local development only, not for network-exposed deployments.

## Privacy controls in the architecture

- Privacy controls are currently enforced in `clipd`, before content is uploaded to the hub.
- Operators can opt in to app-ignore, process-ignore, and sensitive-content filters.
- Those controls are local and best-effort. They do not retroactively delete content that was already synced to other devices or cached elsewhere.

## Why the architecture is centralized

ClipHub intentionally uses a hub-and-spoke design instead of peer-to-peer merge logic:

- clipboard state is naturally last-write-wins,
- clients can reconnect and catch up from a single sequence source,
- the hub can keep short history and status information in one place,
- clients stay simple and only need REST/WebSocket connectivity.

That simplicity comes with an explicit trade-off: the hub is trusted with clipboard contents and metadata. See [Security & Privacy](security.md) for the trust model and [Known Limitations](limitations.md) for behavior and portability caveats.
