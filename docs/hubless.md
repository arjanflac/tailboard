# Hubless tg-clipboard — Exploration

> [!NOTE]
> Historical design record. Native Tailboard apps are clipboard-focused and
> use Taildrop for photos/files; transfer-spool tradeoffs below now apply only
> to the compatibility CLI/backend.

> [!NOTE]
> Historical design record. Native Tailboard apps are clipboard-focused and
> use Taildrop for photos/files; transfer-spool tradeoffs below now apply only
> to the compatibility CLI/backend.

Status: Option 1 implemented; Options 2–3 remain exploration · Companion to [SPEC.md](../SPEC.md) · Last updated: 2026-07-18

## Shipped: embedded hub role

`tg-clipd --embed-hub` carries the existing persistent hub in the agent process. It uses the same SQLite history, transfer metadata, spool quota/TTL, WebSocket stream, and REST protocol as the standalone `tg-clipboard` binary.

```bash
# On the desktop that should carry the role:
tg-clipd --embed-hub

# Equivalent service configuration:
TG_CLIPBOARD_EMBED_HUB=true tg-clipd
```

The default listener binds port `9437` specifically on the machine's Tailscale IP, not on its LAN interfaces. Its state lives under `<tg-clipd state-dir>/embedded-hub/`. Override the listener only for an intentional local/test setup:

```bash
tg-clipd --embed-hub --embed-hub-addr 127.0.0.1:9437
```

Other clients first look for the historic `tg-clipboard` tailnet hostname. If it is absent or unreachable, discovery probes tailnet peers on the role port and accepts only an endpoint whose `/api/capabilities` advertises `hub_role`. `TG_CLIPBOARD_ROLE_PORT` changes the discovery port when the role uses a non-default port.

This removes the dedicated machine, not the broker role. If the role-carrying machine is offline, clipboard sync and asynchronous transfers pause until it returns. Running multiple embedded roles is allowed for manual migration, but automatic election/state handoff is not implemented.

The current architecture requires a dedicated `tg-clipboard` node joined to the tailnet — one more thing to run, keep updated, and keep online. This document explores removing that requirement: **no dedicated hub machine anywhere in the tailnet**, while keeping the properties that make tg-clipboard work (catch-up after reconnect, last-write-wins, the Track C transfer layer).

## What the hub actually provides

Before removing it, list what it does — every design below must answer for each row:

| Hub responsibility | Why it matters |
| --- | --- |
| Single monotonic `seq` | Total order for last-write-wins and `since_seq` catch-up |
| Persistence (SQLite) | History survives restarts; offline devices catch up later |
| Always-on availability | A device that was asleep can fetch the current clip from *somewhere* |
| Fan-out (WebSocket) | One upload, N deliveries |
| Rendezvous identity (`tg-clipboard` hostname) | Zero-config discovery |
| Transfer spool (Track C) | Asynchronous file delivery when the receiver is offline |

The hard ones are **total order**, **always-on availability**, and the **async transfer spool**. Fan-out and discovery are easy without a hub on a tailnet.

## Option 1 — Embedded hub: `tg-clipd --embed-hub` (co-located, not eliminated)

The cheapest reframing: the hub stays in the architecture but stops being a *machine*. `tg-clipd` grows a flag (or auto-mode) that runs the existing hub in-process, serving on the device's own tailnet address via `tailscale serve` or tsnet-free listening on the Tailscale interface.

- Your desktop (or whichever machine is most-often-on) *is* the hub. Other agents discover it the same way they do today, just under a different hostname — or via Option 1b below.
- **Zero protocol changes.** The entire existing stack, including Track C spooling, works unmodified. This is a packaging/UX change, not an architecture change.
- Cost: if the hub-carrying machine is off, sync pauses and history is unreachable — exactly like today when the hub box is off, except now it's a machine you use anyway, so it's likely on when you are.

**1b. Static designation via discovery, not hostname.** Instead of `TG_CLIPBOARD_HOSTNAME=tg-clipboard`, agents read `tailscale status --json` and look for any peer advertising the hub capability (e.g. a well-known tag like `tag:tg-clipboard-hub`, or a port probe against candidate peers). The "hub" becomes a role a device holds, not a node name.

Verdict: **shipped.** It removes the dedicated machine with near-zero engineering risk and is the right default for the single-user tailnet, which is tg-clipboard's core audience.

## Option 2 — Elected hub: the role floats

Same as Option 1, but the hub role moves automatically to whichever eligible device is online.

- Each `tg-clipd --embed-hub=auto` node can serve as hub. Election is deterministic and dumb on purpose: among online eligible peers (visible via the Tailscale local API), the one with the lowest stable node ID wins. Everyone else connects to it. When it disappears, re-run the rule.
- State handoff is the hard part: the new hub starts with an empty (or stale) store. Mitigations:
  - Each embedded hub persists its own copy of history (it is also a subscriber, so it already sees every item — every eligible node keeps a warm replica for free).
  - `seq` cannot survive handoff as a plain counter. Switch the wire ordering key to `(epoch, seq)` — epoch bumps on every election; or use a hybrid logical clock (HLC) per item. Clients catch up with `since_(epoch,seq)`.
- Failure modes to design for: split-brain during a netmap partition (two "lowest IDs" in two partitions — acceptable for a clipboard: partitions can't share clips anyway, and HLC ordering reconciles on heal), and flapping (add a takeover delay).

Verdict: genuinely nice ("it just works whichever machines are on"), but it drags in distributed-systems machinery — epochs, replicas, split-brain — for a product whose data is a clipboard. Worth doing only if Option 1 proves annoying in practice.

## Option 3 — Pure peer-to-peer mesh: no hub role at all

Every `tg-clipd` exposes a small HTTP/WS server on its tailnet address. Peers discover each other from the Tailscale local API (`tailscale status --json` — no mDNS needed, works across networks), form a full mesh (tailnets are small), and gossip clipboard updates directly.

What changes:

- **Ordering:** no central `seq`. Each item carries an HLC timestamp + origin device ID; last-write-wins resolves by `(hlc, device_id)`. This is well-trodden and fine for clipboard semantics — we never merge, we only pick a winner.
- **Catch-up:** a waking device asks any online peer for "items since my last-seen HLC." Every agent already persists its own history locally (a small SQLite each — this replaces the hub DB and incidentally gives *every* device offline-readable history, which is a UX upgrade).
- **All-offline gap:** if you copy on your laptop while every other device is asleep, delivery happens when the next peer comes online and pulls from you — which requires *you* to still be online. Two devices that are never on simultaneously can never sync. The hub's always-on property is genuinely lost; on a two-device tailnet (laptop + phone) this is usually fine, on a "phone → home server while walking out the door" flow it is not.
- **Track C transfers:** direct fetch (SPEC C2 Phase 2) becomes the *only* mode — which is fine for the interactive Blip-style flow (both ends online, progress bar visible) but eliminates asynchronous delivery ("send now, laptop receives when it wakes"). Opportunistic spooling ("any online desktop peer may volunteer to hold the ciphertext") can restore it, but that is real added complexity and a real added trust surface.
- **iOS:** the mesh's weakest link. An iOS device cannot run a listener in the background, so it is a *client-only* peer: it can pull from and push to online desktop peers, but nothing can ever push to it, and it can never serve catch-up. Practically it behaves like today's iOS-with-hub, just pointed at "whichever desktop peer is online" — so iOS needs peer *selection* logic (try peers in last-seen order) instead of one hub URL.

Verdict: architecturally clean and genuinely hubless, but it trades away asynchronous delivery — the property that makes the Track C transfer story better than LocalSend. As a *clipboard-only* mode it is very attractive; as the foundation for transfers it is a downgrade.

## Option 4 — Rejected: piggyback on external always-on state

For completeness: storing the clip in some existing always-on service (Taildrop as a mailbox, a cloud KV, an S3 bucket) reintroduces either a third-party dependency or the very "extra thing to run" we are removing, and breaks the no-cloud promise. Not pursued.

## Comparison

| Property | Today (dedicated hub) | 1: Embedded hub | 2: Elected hub | 3: P2P mesh |
| --- | :---: | :---: | :---: | :---: |
| Extra machine required | yes | **no** | **no** | **no** |
| Works when "main" device is off | yes (hub is on) | no | yes (if any eligible peer on) | partial (pairwise) |
| Async delivery / transfer spool | yes | yes | yes | no (unless volunteered spool) |
| Protocol changes | — | none | epoch/HLC + election | HLC + gossip + per-peer catch-up |
| New failure modes | — | none new | split-brain, handoff | pairwise-never-online gap |
| Engineering size | — | S | L | L–XL |
| iOS story | works | works | works | client-only, peer selection |

## Recommendation

1. **Option 1 (+1b) is the supported hubless deployment.** `tg-clipd --embed-hub` with role-based discovery removes the dedicated machine for the common case. Pick one machine to carry the role (usually a desktop), or keep a dedicated node if you want always-on behavior.
2. **Prototype Option 3 as a clipboard-only degraded mode**, not a replacement: when no hub-role peer is reachable, agents that support it exchange clips directly (HLC-ordered) and reconcile with the hub when it returns. This gives "my laptop and desktop still sync at the coffee shop while the home hub is unreachable" without betting the architecture on the mesh.
3. **Do not pursue Option 2** unless Option 1's manual designation proves to be a recurring support burden.
4. **Keep the hub role mandatory for Track C spooled transfers.** Transfers between two online devices can already go direct (C2 Phase 2); asynchronous transfers inherently need *something* durable and online, and an embedded hub on your most-on machine is the honest minimum.

## Impact on SPEC.md if adopted

- Track A gains A7: embedded hub mode + role-based discovery (Phase 1–2 work).
- Architecture docs change the topology claim from "a broker runs inside your tailnet" to "one of your devices carries the broker role."
- Open question 3 in SPEC.md (hub on small devices) partially dissolves: the spool lives on whichever real machine carries the role.
- New open question: does the HLC/epoch ordering key ship in the wire protocol *now* (cheap while clients are few, enables Options 2/3 later) even if only Option 1 ships? Leaning yes — add HLC alongside `seq`, ignore it until needed.
