# Interface Spec: A LocalSend/Blip-Style Experience for ClipHub

Status: **Proposed**
Scope: macOS `clipd` companion UI, iOS ClipHub app, shared vocabulary and onboarding.
Non-scope: protocol changes beyond small control-API additions, E2EE (tracked in `docs/e2ee-transfers.md`), Windows/Linux GUIs (follow the same principles later).

---

## 1. Why

Today the two interfaces sit at opposite extremes:

- **macOS `clipd` has effectively no UI.** The only surface is an embedded 81-line HTML page opened in a browser via `--tray`, plus `osascript` notifications that tell users to *run a CLI command* ("run `tailclip receive --id X`"). Everything else is flags and env vars. `docs/limitations.md` already admits: "does not yet install a native menu-bar icon."
- **iOS ClipHub has a real app**, and the `apple-design-redesign` branch has already humanized copy, componentized views, and added motion/a11y polish — but the product model it presents is still an infrastructure diagram: users type a **Hub URL** (`http://100.x.x.x`), read raw WebSocket disconnect reasons, and see "hub" as a first-class concept ("Push clipboard to ClipHub", "Change Hub…").

LocalSend and Blip win on a simple promise: **open the app, see your devices as people-like tiles, drop a thing on one, it arrives.** No addresses, no server concept, no states you have to interpret. That is exactly the experience our plumbing can already support — Tailscale-based auto-discovery exists (`internal/discover`), devices self-register, transfers are resumable with accept/decline — we just never built the front door.

**Thesis:** we don't need new infrastructure to feel like LocalSend. We need to (a) put *devices* at the center of both UIs, (b) delete the "hub" from the user's mental model, and (c) make the desktop a native, always-visible citizen.

---

## 2. Design principles

1. **Devices, not infrastructure.** The primary noun in every screen is a *device* ("Thalys's MacBook"), never a hub, URL, socket, or daemon. The hub is an implementation detail on par with a DNS server.
2. **Zero-config by default.** Discovery already works (`tailscale status` → probe port 9437 → capability check). Manual URL entry becomes the escape hatch, not step one of onboarding.
3. **The verb is "send", the gesture is "drop".** Files and clips are sent by dropping onto or tapping a device tile — the LocalSend interaction. "Push to hub" disappears as a concept; you send *to your other devices*.
4. **States are human.** Three user-visible connection states only: **synced / connecting / offline**, plus one actionable condition: **"Tailscale is off"** with a button. Raw `error.localizedDescription` never reaches a label.
5. **Quiet until needed.** Clipboard sync is ambient. UI appears for: incoming transfer offers, failures that need a decision, and explicit user intent (opening the popover/app).
6. **Same product, two idioms.** macOS is a menu bar utility (glanceable, drag-first). iOS is the full app (history, settings, extensions). Both share vocabulary, iconography, status semantics, and the teal accent already in `Assets.xcassets`.

---

## 3. Shared vocabulary (both platforms)

| Today (banned from UI) | Spec |
|---|---|
| "Hub", "Hub URL", `http://100.x.x.x`, MagicDNS | *(invisible — shown only inside an "Advanced" screen as "Sync server")* |
| "Disconnected: \<raw error\>" | "Offline — reconnecting…" |
| "VPN Required" | "Tailscale is off" + **Open Tailscale** button |
| "Push clipboard to ClipHub" | "Send clipboard" (to a device or to all) |
| "Devices register with the hub" | "Your devices appear automatically" |
| Raw device ID fallback | "Unknown device" |
| `[image/png, 12345 bytes]` | "Image · 12 KB" (extend `displaySummary` to every preview path, incl. keyboard/Live Activity) |
| "Transfer offered/accepting/spool…" | "Waiting for \<device\>…", "Receiving…", "Saved to Downloads" |

Status semantics (shared): a device is **online** (green dot, `StatusDot` already never color-only), **away** ("Last seen 5 min ago"), or absent. The *connection* is **synced / connecting / offline / Tailscale off**. Nothing else.

---

## 4. macOS: `ClipHub.app` menu bar companion

A new SwiftUI `MenuBarExtra` app (`ios/` tree gains a macOS target, or a sibling `macos/` directory) that is a **pure client of clipd's loopback control API** (`127.0.0.1:9438`). clipd remains the headless engine; the app supervises it.

### 4.1 Menu bar item

- Icon: clipboard glyph. States: normal (synced), animated/dimmed (connecting), badge dot (pending incoming offer), slash (paused), warning (Tailscale off / clipd not running).
- **Drag-and-drop onto the menu bar icon itself** opens the popover in "pick a device" mode with the dragged files staged — the fastest LocalSend-style path.

### 4.2 Popover layout (top to bottom)

1. **Current clip card** — same `displaySummary` humanization as iOS ("Link · github.com/…", "Image · 2.3 MB"), with Copy button and relative time ("from iPhone · 2 min ago").
2. **Device grid** — the centerpiece. One tile per device (SF Symbol per platform, name, status dot). Each tile is:
   - a **drop target** for files/folders → `POST /api/send?to=<id>` (multipart, already supports multi-file; folders already tar deterministically),
   - clickable → small action sheet: *Send files…* (file picker), *Send clipboard*, *device details*.
3. **Activity section** — in-flight and recent transfers with progress bars (poll `/api/state`; see 4.4 for push), Accept/Decline buttons inline for offers, "Show in Finder" on completion (`POST /api/reveal` exists; extend to reveal the specific file).
4. **Footer** — pause/resume toggle (`POST /api/pause`), Settings…, and a single status line ("Synced · 3 devices").

### 4.3 System integration

- **Native notifications** (UNUserNotificationCenter) replace `osascript`: incoming offer notifications get **Accept / Decline action buttons** (mapped to `POST /api/transfers/{id}/accept|decline`) — no more "run tailclip receive". Completion notification click reveals the file.
- **Finder Share/Services menu**: "Send with ClipHub" quick action → device picker → same send path.
- **Login item + supervision**: the app installs/starts the clipd launch agent on first run (reusing `internal/service/install.go` semantics), shows "ClipHub engine isn't running — Start" when the control port is unreachable.
- First run = the onboarding in §6; ships with `--embed-hub` decision made *for* the user (see 6.2).

### 4.4 Required clipd control-API additions

Small, all loopback-only, keeps the app dumb:

| Addition | Why |
|---|---|
| `GET /api/events` (SSE or WS on the control port) | popover progress without 1 s polling; offer badge updates |
| `GET/PUT /api/settings` — privacy preset, ignore-apps/processes, sensitive classes, transfers policy (`ask/accept/off` + allowlist), download dir, device name | today these are **start-time flags only**; a GUI needs runtime mutation. clipd persists to a new `config.json` in state-dir and applies live (privacy policy and transfer policy are already consulted per-event, so hot-reload is cheap) |
| `POST /api/pause` unified with the pause-file | today the file flag (`tailclip pause`) and the in-memory flag are separate; the API should read/write both so CLI and GUI agree |
| `GET /api/state` gains `connection` (`synced/connecting/offline/no-tailscale`) and `hub` info (for the Advanced screen) | the app must render principle-4 states without guessing |
| `POST /api/clip` (set clipboard content / "send clipboard now") | explicit send-clipboard action |

### 4.5 Settings window (the *only* place plumbing is visible)

- **General**: device name, launch at login, download folder.
- **Privacy**: preset picker (Strict / Balanced / Off) with plain-language descriptions, ignored-apps list (drag an app in), "filter passwords & one-time codes" toggles, clear-on-block.
- **Receiving**: Ask every time / Auto-accept from my devices (allowlist) / Off.
- **Advanced** (collapsed): sync server address override, "act as sync server for my devices" (`--embed-hub`), port, open web console (the existing control.html survives as a debug page).

---

## 5. iOS: from dashboard to LocalSend

The redesign branch's component work (`Banners`, `StatusDot`, `ConfirmingActionButton`, motion tokens) is the foundation; this spec re-aims the information architecture on top of it. Tabs stay three, but re-centered:

### 5.1 Tab 1 — **Clipboard** (keep, refine)

Stays the home tab, mostly as redesigned. Changes:
- Connection banner uses the 4 human states only; "Tailscale is off" banner gets an **Open Tailscale** deep link (`tailscale://`).
- "Push iPhone Clipboard" → **"Send clipboard"**; primary action row becomes Copy / Send / Share.
- Kill every remaining raw preview: `ClipItem.preview`'s `[image/png, 12345 bytes]` path is replaced by `displaySummary` everywhere (keyboard chips, Live Activity).

### 5.2 Tab 2 — **Devices** (becomes the LocalSend heart)

- **Tile grid**, not a list: platform glyph, name, status dot / "5 min ago". Visually the sibling of the macOS popover grid.
- **Tap a device → send sheet**: "Send clipboard", "Send photos…" (PhotosPicker), "Send files…" (document picker). This adds *in-app file sending* — today sending only exists in the share extension; the hub API (`POST /api/transfers` + chunked `PUT`) already supports it, so this is client work only.
- **Incoming offers** stay at the top of this tab *and* arrive as actionable push-style banners with Accept/Decline (reuse `TransferRowView`), with per-file progress and "Saved — open in Files".
- Empty state gains an install pointer: "No devices yet. Install ClipHub on your Mac to get started" → link to the repo/site (mirrors LocalSend's "open it on the other device" framing).

### 5.3 Tab 3 — **Settings** (de-plumbed)

- Connection block shows: state (human), *this device's name* (editable — re-registers), and device list shortcut. Hub URL moves into **Advanced › Sync server**, still monospace, still copyable — for debugging, not identity.
- Keyboard-setup instructions stay (they're good); add the same "Receiving files" policy picker as macOS when clipd grows the settings API (per-device policy is hub/agent-side).

### 5.4 Extensions

- Share extension is already the closest thing we have to LocalSend — keep the flow, rename the destination "Tail Clipboard" → **"All my devices (clipboard)"** vs. named devices, and adopt the tile visual for the device picker.
- Keyboard: no structural change; copy fixes from §3 apply.

---

## 6. Onboarding: zero-config first

### 6.1 iOS (replaces the URL-entry screen)

1. **Welcome** — icon + one line: "Your clipboard and files, on every device. Private, over Tailscale."
2. **Looking for your devices…** — the app runs discovery itself: query the Tailscale local API/app for peers, probe `:9437` `/api/capabilities` for `hub_role`, and probe hostname `cliphub` — the same algorithm as `internal/discover`, ported to Swift. Spinner ≤ 5 s.
   - **Found** → "Found your devices ✓" + tile preview of the roster → **Get started**. Hub URL is stored silently.
   - **Tailscale not running/installed** → explanatory card + "Open Tailscale" / App Store link, retry on foreground.
   - **Nothing found** → "Set up your Mac first" card (link to install instructions) + small **"Enter address manually"** escape hatch = the current screen, demoted.
3. **Device name** — prefilled "Thalys's iPhone" (`UIDevice.name`), single field, Continue.
4. Keyboard/Live Activity setup offered as skippable cards, not blockers.

### 6.2 macOS first run

1. Drag-install app → open → "Start syncing" (installs/starts the clipd agent).
2. Discovery: if a hub is found → join it. If not → **silently enable `--embed-hub`** on this machine ("Your Mac will keep your devices in sync"), so the first desktop bootstraps the network with zero questions. The hubless design (`docs/hubless.md` Option 1) already ships this; onboarding just makes the decision automatic.
3. Show a QR-less, code-less finish screen: "Now install ClipHub on your phone — it will find this Mac automatically." (Tailnet membership *is* pairing; unlike LocalSend we never need PINs.)

---

## 7. Phasing

| Phase | Deliverable | Depends on |
|---|---|---|
| **1. Vocabulary & states (iOS)** | §3 copy table, 4-state connection model, kill raw previews/errors, Devices tile grid | nothing — pure client work on the redesign branch |
| **2. Zero-config onboarding (iOS)** | §6.1 discovery flow, URL demoted to Advanced | Swift port of discovery probing |
| **3. macOS menu bar app MVP** | §4.1–4.3: popover, device drop targets, native notifications with actions, pause | existing control API only |
| **4. clipd settings/events API** | §4.4 additions + `config.json` persistence | Go work; unblocks Settings UIs on both platforms |
| **5. In-app sending (iOS)** | §5.2 send sheet (photos/files) | hub transfer API (exists) |
| **6. Polish** | Finder quick action, macOS onboarding w/ auto embed-hub, iOS receiving-policy UI | 3 + 4 |

Phases 1–3 alone get us to "feels like LocalSend"; 4–6 make it complete.

---

## 8. Explicitly out of scope

- **E2EE transfers** — design exists (`docs/e2ee-transfers.md`), deferred; the UI should avoid claiming end-to-end encryption until then ("Private, over Tailscale" is accurate).
- **PIN/QR pairing** — unnecessary; tailnet membership is the trust boundary. Revisit only if we ever support non-Tailscale transports.
- **Windows/Linux tray apps** — same principles, later; the control-API additions in §4.4 are designed to serve them unchanged.
- **Conflict resolution / selective sync** — see `docs/limitations.md`; unchanged by this spec.
