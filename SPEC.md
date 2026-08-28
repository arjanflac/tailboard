# tg-clipboard SPEC — Finishing the Vision, iOS, and the Tailnet Transfer Layer

Status: implemented · Last updated: 2026-07-18

## Implementation outcome

Tracks A–C and the recommended embedded-hub option from `docs/hubless.md` are implemented in the repository. The shipped result includes:

- native rich Windows clipboard support and event-driven Windows/Wayland watching,
- full desktop/ARM64 release artifacts, installable services, and configurable tap/bucket publication,
- privacy presets plus layered foreground detection and local audit output,
- stable devices/capabilities, maintained iOS app/extensions/widgets/Intents, simulator CI, and a credential-gated TestFlight workflow,
- durable resumable spool transfers, receiver consent/allowlists, iOS and desktop device surfaces, and capability-gated direct fetch,
- `tg-clipd --embed-hub` plus capability-based role discovery, removing the dedicated-machine requirement.

Two boundaries remain intentional and documented rather than silently claimed:

1. `tg-clipd --tray` opens the shipped loopback-only cross-platform desktop companion in the default browser. A native menu-bar/taskbar wrapper is optional follow-up polish; the device/drop/progress/consent/pause feature surface itself is complete.
2. External Homebrew/Scoop repositories and TestFlight uploads run only after their repository variables/secrets are configured. The workflows are implemented and locally validated, but this source change does not create third-party repositories or publish an Apple build.

Transfer E2EE remains deferred exactly as scoped. The transfers-first protocol decision, algorithms, downgrade rule, metadata leakage, and key-lifecycle gates are recorded in `docs/e2ee-transfers.md`; no current UI claims E2EE.

This SPEC covers three tracks:

1. **Track A — Finish the original vision.** Close the gaps the roadmap already names between what the docs promise and what ships.
2. **Track B — iOS as a supported platform.** Turn the source-only iOS companion into a packaged, honest, first-class client.
3. **Track C — Beyond the clipboard.** Extend tg-clipboard into a LocalSend alternative over the tailnet: targeted device-to-device transfer of files and payloads of arbitrary size.

The tracks are ordered by dependency: Track C reuses the device registry and transport work from Tracks A/B, so the sequencing at the end of this document interleaves them deliberately.

---

## Where the project actually stands (baseline audit)

What exists and works today:

- **Hub (`tg-clipboard`)** — tsnet node with auto-HTTPS, SQLite-backed history, monotonic sequence numbers, WebSocket fan-out with `since_seq` catch-up, raw blob endpoints, cursor-paged history, typed errors, health/readiness/metrics endpoints, graceful shutdown.
- **Agent (`tg-clipd`)** — 500 ms polling watcher, cross-device MIME selection (png → plain → HTML-only fallback), hash+MIME dedup, echo-loop prevention via read-back, opt-in privacy filters (app/process ignore lists, `secret`/`password-manager`/`otp` content classes, clear-on-block), pause/resume.
- **CLI (`tg-clip`)** — get/put/history/status/clear/pause/resume, file put with MIME detection, stdin piping.
- **Release engineering** — deterministic archives, checksums, Homebrew/Scoop/winget metadata generation, CI on three OSes with a race gate.
- **iOS companion (~1,400 lines Swift, xcodegen project)** — container app (current clip, history, settings, onboarding), `TGClipboardKit` framework (REST client, WebSocket manager, app-group cache), TailPaste keyboard (inserts current/recent hub clips), share extension (sends content to hub). Manual signing, no CI, no packaging.

The honest gaps, from the repo's own docs:

| Gap | Where documented |
| --- | --- |
| Windows clipboard is `text/plain` only (PowerShell cmdlet backend) | platform-support.md |
| iOS is source-first, unpackaged, no CI, narrower interaction model | limitations.md, platform-support.md |
| Hub release artifact is `linux/amd64` only despite broader source support | limitations.md |
| No E2E encryption; hub reads and stores raw content | security.md |
| No per-device permissions or selective sync | security.md |
| Polling, not event-driven, clipboard watch | limitations.md |
| Files explicitly out of scope ("use Taildrop") | README, roadmap.md |
| 10 MiB payload cap in the protocol | limitations.md, `protocol.MaxContentSize` |

---

## Track A — Finish the original vision

Goal: make every claim in the README true on every supported platform, and remove the "supported with reduced fidelity" asterisks.

### A1. Windows clipboard parity (highest-value single fix)

The PowerShell-cmdlet backend is the weakest part of the desktop stack — slow (process spawn per poll), lossy (plain text only), and fragile.

- Replace `internal/clipboard/clipboard_windows.go` with direct Win32 clipboard API access via `golang.org/x/sys/windows` (`OpenClipboard`/`GetClipboardData`/`SetClipboardData`). No cgo required.
- Support `CF_UNICODETEXT` (text), `CF_HTML` (registered `HTML Format` with its header envelope), and `CF_DIB`/`CF_PNG` for images, converting to/from the canonical `image/png` wire format.
- Replace polling with event-driven watch on Windows using `AddClipboardFormatListener` + a message-only window; keep the poll loop as fallback.
- Acceptance: platform-support matrix shows yes/yes/yes for Windows; round-trip tests for each MIME type run in the Windows CI leg.

### A2. Event-driven clipboard watching everywhere it is possible

- macOS: keep polling `NSPasteboard.changeCount` (there is no change notification API), but poll the cheap `changeCount` integer instead of reading content each tick — read content only when the count moves.
- Linux Wayland: use the `wlr-data-control` / `ext-data-control` protocol for event-driven watch where the compositor supports it; fall back to `wl-paste --watch`.
- Windows: covered by A1.
- Acceptance: idle `tg-clipd` performs no content reads and no subprocess spawns when the clipboard hasn't changed.

### A3. Release and packaging parity

- Cross-build the hub for all platforms `tg-clipd` ships for (darwin/arm64+amd64, linux/amd64+arm64, windows/amd64). tsnet is pure Go; there is no technical blocker.
- Add linux/arm64 across all binaries (Raspberry Pi hub is a natural deployment).
- Ship the launchd/systemd units inside the release archives and add `tg-clipd install-service` / `tg-clipboard install-service` subcommands that write and load them. This is the single biggest onboarding-friction fix.
- Publish the Homebrew tap and Scoop bucket as real repositories (metadata generation already exists; the last mile is the tap repos and a release-workflow push step).

### A4. Privacy-control hardening (roadmap's second bullet)

- Replace the `xdotool` dependency on Linux with a layered detector: native Wayland foreground-window protocols where available → X11 EWMH via a small pure-Go xgb client → `xdotool` as last resort. Report which layer is active in `tg-clipd` logs and `tg-clip status`.
- Add a `--privacy-preset strict|balanced|off` flag so users get sane bundles without learning four flags. `strict` = all sensitive classes + clear-on-block.
- Emit a local (never synced) audit line when a rule blocks an item, so users can verify filters actually fire.
- Document per-platform detection coverage as a table in security.md.

### A5. Protocol/versioning groundwork (prerequisite for B and C)

- Add `GET /api/capabilities` returning hub version, protocol version, feature flags (`transfers`, `devices`, `e2ee`), and limits (max clip size, max transfer size). Clients gate features on this instead of sniffing 404s.
- Add a `device_id` (stable UUID per install) alongside the human-readable `source` on every message. Persist in agent state. This is the seed of the device registry Track C needs.
- Keep the wire format backward compatible: old clients ignore unknown fields; the hub tolerates missing `device_id`.

### A6. Explicit non-goals for Track A (unchanged from roadmap)

- No multi-hub replication or HA.
- No conflict resolution beyond last-write-wins.
- E2E encryption stays out of Track A; it is revisited as an open question in Track C, where transfers raise the stakes.

---

## Track B — iOS: app + keyboard, packaged and honest

### B1. Product shape decision: **app + keyboard + share extension** (keep the trio, deepen each)

A keyboard-only product cannot work, and it's worth recording why so the decision doesn't get relitigated:

- iOS has **no background clipboard read**. Since iOS 14/16, reading `UIPasteboard` triggers a user-visible banner and (since 16) a permission prompt, and only a foregrounded process can do it at all. A desktop-style always-on watcher is impossible; nothing in Track B should promise "copy on iPhone, it appears on your Mac automatically."
- A keyboard extension is the only surface that can **insert text into any app** without leaving it — that's the paste half.
- A share extension is the best surface for the **send half** (share sheet from any app, no clipboard involved).
- The container app is required anyway (keyboards/extensions must ship inside an app), and it's the right home for history browsing, settings, and the receive side of Track C.

So the iOS interaction model, stated honestly:

| Direction | Surface | Trigger |
| --- | --- | --- |
| Hub → iPhone (paste) | TailPaste keyboard | User switches keyboard, taps a clip |
| Hub → iPhone (copy to local clipboard) | App, widget, Shortcuts intent | User taps "Copy" |
| iPhone → Hub (send) | Share extension | User shares from any app |
| iPhone → Hub (send clipboard) | App foreground, Shortcut/Action button, keyboard "push" button | User-initiated; clipboard read prompts apply |

### B2. Fill the gaps in the existing iOS code

The Swift code is a solid skeleton; these are the concrete deltas:

1. **Keyboard: send as well as paste.** Add a "Push clipboard" key that reads `UIPasteboard` (Full Access already required) and POSTs it to the hub. This makes the keyboard bidirectional and is the closest iOS can get to desktop `tg-clipd`.
2. **Images and rich content.** The keyboard currently filters to text. Support image clips: render a thumbnail, and on tap copy the image to the local pasteboard (keyboards can't insert images into the text proxy; copy-to-pasteboard is the correct fallback). Share extension should accept images, URLs, and files via `NSItemProvider` type identifiers and use the raw `/api/clip/blob` endpoint instead of base64 JSON.
3. **App Intents / Shortcuts.** Expose `GetCurrentClip`, `PushClipboard`, `SendToHub(text/file)` as App Intents. This unlocks the Action button, Back Tap, and user automations — the pragmatic substitute for a background watcher.
4. **Widgets + Live Activity.** A lock-screen/home widget showing the current hub clip (age + preview) with a tap-to-copy deep link. Cheap to build on the existing app-group cache.
5. **Connection layer hardening.** The WebSocket manager should be foreground-only with reconnect + `since_seq` catch-up on `scenePhase` changes; use `URLSession` background configuration for share-extension uploads so large sends survive the extension's short lifetime.
6. **Keychain, not JSON.** Move hub URL and any future secrets from plain app-group JSON into an app-group Keychain item (security.md currently has to apologize for this).

### B3. Networking/transport on iOS

- Keep the current requirement: the **Tailscale app provides the tunnel**; tg-clipboard speaks plain HTTPS to `https://tg-clipboard.<tailnet>.ts.net`. Detect "tailnet unreachable" and show a "Open Tailscale" fix-it button rather than a generic error.
- Do **not** embed libtailscale/tsnet in the iOS app for now: it would conflict with the user's existing Tailscale VPN profile (iOS allows one active packet tunnel), balloon the binary, and complicate App Store review. Revisit only if a keyboard-without-VPN story becomes critical.
- Note the sharp edge: **keyboard extensions can reach the network only with Full Access**, and traffic still flows through the Tailscale tunnel. Document that the keyboard is fully functional offline from cache (already implemented) and degrades gracefully.

### B4. Packaging and support tier

- Add an `ios-ci` GitHub Actions job on a macOS runner: `xcodegen generate` + `xcodebuild build test` (simulator, no signing). This alone moves iOS from "source dump" to "maintained."
- Distribute via **TestFlight** as the supported path (personal team sideloading stays documented as the free alternative). App Store submission is a stretch goal — the keyboard's Full Access requirement invites review friction, and tg-clipboard-on-a-tailnet is inherently a self-hosted-audience product.
- Update platform-support.md: iOS becomes "supported companion (interaction model differs from desktop by OS design)" with the table from B1 as the canonical capability statement.

### B5. iOS acceptance criteria

- A user with Tailscale + TestFlight can go from install to first paste in under five minutes with no Xcode.
- Copy on desktop → open iPhone keyboard → clip is present (via refresh) — offline shows the cached clip.
- Share a photo from Photos → arrives in desktop clipboard as PNG.
- Action-button Shortcut pushes iPhone clipboard to the hub.

---

## Track C — From clipboard sync to a LocalSend alternative

### C1. Product definition

Add a second primitive next to the clipboard: **transfers** — explicit, targeted, size-unbounded delivery of files (or file sets) from one device to another over the tailnet.

What we take from LocalSend: pick a nearby device, drop files on it, no cloud, works for gigabytes. What we do differently (and better, given the tailnet):

- **No discovery pain.** LocalSend needs mDNS on a shared LAN. tg-clipboard devices already share a network (the tailnet) and will already be registered with the hub — device discovery is a hub query, and it works across networks (phone on LTE → home server), which LocalSend cannot do.
- **No pairing ceremony.** Tailnet membership is the auth, same trust model as the clipboard.
- **Asynchronous by default.** LocalSend requires both devices online simultaneously. With the hub spooling transfers, "send now, receive when the laptop wakes up" works. Direct P2P remains an optimization, not a requirement.

Clipboard sync and transfers stay distinct primitives: the clipboard is broadcast, implicit, ephemeral, small; transfers are targeted, explicit, durable-until-claimed, large. Don't blur them (e.g., don't auto-copy received files into the clipboard).

### C2. Architecture: hub-spooled with direct-fetch optimization

**Phase 1 (spooled, ship first):** sender uploads to the hub; hub stores the transfer in a spool directory; hub notifies the target device over the existing WebSocket; receiver downloads and acks; hub deletes the spool (or lets TTL expire it).

Why hub-first rather than P2P-first: it reuses every mechanism tg-clipboard already has (WebSocket fan-out, tsnet identity, sequence/catch-up thinking), it gives asynchronous delivery for free, and both endpoints only ever speak client→hub HTTPS, which is the only thing iOS extensions can reliably do.

**Phase 2 (direct, optimization):** when both devices are desktop agents and online, the hub can broker a direct fetch — sender's `tg-clipd` exposes a one-shot authenticated download endpoint on its tailnet address, receiver pulls directly, hub only carries the offer/ack metadata. Cuts the double-hop for multi-GB transfers. Gated behind the `capabilities` flag; falls back to spooling transparently.

### C3. New hub surface

Device registry (also serves Track B/status UX):

```
POST /api/devices/register     {device_id, name, platform, capabilities}
GET  /api/devices              → [{device_id, name, platform, online, last_seen}]
```

`online` is derived from live WebSocket connections; registration happens implicitly on agent connect.

Transfers:

```
POST   /api/transfers                          create: {to_device, files:[{name, size, mime, sha256}], note?} → {transfer_id, upload_urls}
PUT    /api/transfers/{id}/files/{n}           chunked/resumable upload (Content-Range; hub verifies sha256 on completion)
GET    /api/transfers?role=receiver&state=offered   list pending
POST   /api/transfers/{id}/accept | /decline
GET    /api/transfers/{id}/files/{n}           download (Range supported)
POST   /api/transfers/{id}/complete            receiver ack → hub deletes spool
DELETE /api/transfers/{id}                     cancel (either side)
```

WebSocket gains message types alongside `clip`: `transfer_offer`, `transfer_state` (accepted/declined/progress/complete/canceled). Existing clients ignore unknown types (verify and lock this in with a test).

Transfer state machine: `offered → accepted → transferring → complete` with exits to `declined | canceled | expired`. Spool TTL default 48 h (separate from clip TTL); expired transfers notify the sender.

Storage: spool as files on disk (`~/.config/tg-clipboard/spool/{transfer_id}/`), metadata in SQLite. **Not** in SQLite blobs — transfers are orders of magnitude beyond the 10 MiB clip cap. Hub enforces a configurable spool quota (`--spool-quota`, default e.g. 10 GiB) and per-transfer max size, both advertised via `/api/capabilities`.

### C4. Client experience

**UX reference: [Blip](https://blip.net).** Blip is the bar for how transfers should feel, and it dictates four principles for every surface below:

1. **Devices are the UI.** The primary screen is a list of your devices (name, platform icon, online dot) — you act on a *device*, not on a "transfer form." The device registry (C3) is what makes this renderable.
2. **Drag-and-drop, then walk away.** Sending is: drop files on a device, done. Progress is visible but never modal; transfers survive app restarts and network blips (resumable uploads in C3 are a UX requirement, not just robustness).
3. **No visible size ceiling.** Multi-GB sends must feel routine — chunked/resumable transport, clear progress with rate/ETA, pause/resume.
4. **Receiving is calm.** An incoming offer is one lightweight prompt (or silent, from allowlisted own-devices), lands in a predictable folder, and ends with a "Show in folder" affordance.

Blip's model implies something tg-clipboard doesn't have yet: a **desktop GUI surface**. `tg-clipd` grows a menu-bar/tray companion (Phase 3–4): device list with drag-and-drop targets, active transfer progress, incoming-offer prompts, and clipboard pause/resume for good measure. Keep it a thin shell over the existing agent — `tg-clipd` stays headless-capable, the tray talks to it locally (extend the agent with a small local IPC/HTTP surface). CLI remains the scriptable path and ships first; the tray is what makes it feel like Blip. Where tg-clipboard deliberately differs from Blip: no accounts and no cross-user "friends" — the tailnet is the roster.

CLI (`tg-clip`):

```
tg-clip devices                         # list tailnet devices + online state
tg-clip send file.pdf --to laptop      # spool, notify, exit (or --wait for ack)
tg-clip send ./dir --to phone           # directories: tar or per-file set
tg-clip transfers                       # pending in/out
tg-clip receive [--id N] [--to DIR]     # accept + download (default ~/Downloads)
```

`tg-clipd` (desktop agent):

- Listens for `transfer_offer`; policy flag `--transfers accept|ask|off` (default `ask`).
- `ask` surfaces a native notification ("iPhone wants to send photo.jpg (4.2 MB) — Accept / Decline") via osascript/notify-send/toast; accepted files land in `~/Downloads` (configurable), with a completion notification that can open the containing folder.
- `accept` mode auto-downloads from an allowlist of device IDs — the "drop files on my own machines without touching anything" flow, which is the 90% personal use case.

iOS (Blip-style: the device list is the home screen):

- Container app's main tab becomes the device list; tapping a device opens a drop target / recent-transfers view. Clipboard current/history moves to a second tab.
- Share extension gains a device picker (share → tg-clipboard → choose "MacBook" instead of "clipboard"), using background `URLSession` uploads so large sends survive the extension lifetime.
- Inbox for incoming offers: accept/decline, downloads to the app's Documents (visible in Files), share-sheet re-export. Foreground-only receive is acceptable; a push-notification receive path is an explicit non-goal until someone runs a push relay.
- App Intent `SendFileToDevice` for Shortcuts.

### C5. Security posture for transfers

- Same trust boundary as the clipboard: tailnet membership authorizes; the hub sees transfer contents (spooled mode). State this as bluntly as security.md states it for clips.
- Receiver consent (`ask` default) is the key difference from clipboard broadcast — transfers are the surface where an annoying tailnet member could dump junk on your disk, so consent + per-device allowlists ship in v1, not later.
- Integrity: per-file sha256 declared at create time, verified by hub on upload completion and by receiver on download.
- Path safety: receiver sanitizes filenames (no separators, no `..`, no overwriting without `--force`/prompt).
- **Open question — E2EE:** transfers make hub-readability heavier (documents, not snippets). A tractable v2 design: sender encrypts per-transfer with a random key, wraps the key for the target device's registered public key (device keypair generated at first run, pubkey stored in the registry); the hub spools ciphertext. This is much easier for targeted transfers than for broadcast clipboard sync, so E2EE may land in transfers *first*. Deferred, but the device-registry schema should reserve a `public_key` column now.

### C6. Explicit non-goals for Track C

- No non-tailnet mode (no LAN mDNS discovery, no QR-code pairing with strangers). Tailscale's own Taildrop and LocalSend serve those; tg-clipboard's differentiation is the always-on hub inside a network you already trust.
- No Android client in this SPEC (the protocol is plain HTTPS+WS, so nothing precludes one later).
- No transfer history/archive; like the clipboard, the hub spool is a conveyor belt, not storage.

---

## Sequencing

| Phase | Contents | Rationale |
| --- | --- | --- |
| 1 | A5 (capabilities + device_id), A3 (packaging/services), A1 (Windows parity) | Protocol groundwork everything else gates on; biggest adoption friction removed |
| 2 | B2/B3/B4 (iOS deltas, CI, TestFlight), A2, A4 | iOS becomes a real product on the improved base |
| 3 | C2-Phase-1 + C3 + C4 CLI/desktop (spooled transfers) | The LocalSend-alternative core, desktop-first |
| 4 | C4 iOS (device-list home, share-to-device, Inbox), desktop tray companion, widgets/intents polish | The Blip-grade UX layer on both ends |
| 5 | C2-Phase-2 (direct fetch), E2EE-for-transfers exploration | Optimizations and posture upgrades |

Each phase ends with docs updated (platform-support matrix, security.md, limitations.md) — this repo's discipline of documenting the honest support boundary is one of its best features; keep it.

## Open questions

1. **E2EE scope** — transfers-first (C5) or never? Decide before the device registry ships so the schema reserves key material fields.
2. **Directory semantics** — tar-on-send (simple, opaque) vs. per-file set (resumable, browsable)? Leaning per-file set with a manifest.
3. **Hub spool on small devices** — if the hub runs on a Pi, a 4 GB video exceeds sensible spool quotas; is "direct fetch or fail" acceptable there, or do we need streaming pass-through (hub pipes sender→receiver without landing on disk) as a middle mode?
4. **App Store vs TestFlight-only** for iOS — pursue review (Full Access keyboard + VPN-adjacent product) or accept TestFlight as the ceiling?
5. **Rename?** "tg-clipboard" undersells a product that also moves files. Worth deciding before public positioning, painful after.
6. **Hubless operation** — can the dedicated `tg-clipboard` machine be removed entirely? Explored separately in [docs/hubless.md](docs/hubless.md); its recommendation (embedded hub role in `tg-clipd`, role-based discovery) would land as a new Track A item.
