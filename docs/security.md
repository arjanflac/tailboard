# Security & Privacy

See also: [Architecture](architecture.md), [Known Limitations](limitations.md), [Platform Support](platform-support.md), [Roadmap](roadmap.md), [README](../README.md), [SECURITY.md](../SECURITY.md)

tg-clipboard is a sensitive product because it moves clipboard contents across devices. The security posture is intentionally simple: trust your tailnet, trust the hub you run inside it, and trust every device you connect to that hub.

## Security model

- The normal deployment boundary is your Tailscale tailnet.
- In tailnet mode, `tg-clipboard` identifies callers through Tailscale/tsnet rather than separate tg-clipboard accounts or API tokens.
- Every connected client that can reach the hub can receive synced clipboard content.
- The hub is not a blind relay. It reads, stores, and replays clipboard data.

## Privacy posture

- Clipboard contents are stored on the hub in SQLite so reconnecting clients can catch up and history can survive restarts.
- By default, raw clipboard data remains on disk for up to 24 hours and up to 50 history items.
- Content hashes are used for deduplication, but the raw content is also retained because clients need the original payload.
- Clipboard contents are also exposed to the destination device's local clipboard and whatever OS or app integrations that device already permits.
- Non-secret iOS clip cache data lives in the shared app-group container. Hub configuration and future key material use an access-group Keychain item.

## Current privacy controls

tg-clipboard now includes opt-in privacy controls on the desktop agent:

- `--ignore-apps` / `--ignore-processes` keep matching foreground contexts local.
- `--filter-sensitive` can block `secret`, `password-manager`, and `otp` classes.
- `--clear-on-block` can clear the local clipboard when a privacy rule blocks sync.
- `--privacy-preset strict|balanced|off` provides reviewed policy bundles. Strict enables every sensitive class plus clear-on-block.
- Every blocked item emits a local audit log entry; audit details are never uploaded.
- `tg-clip clear` removes hub clipboard state and persisted history, and `tg-clip clear --local` also clears the invoking machine's clipboard.

Foreground detection is best-effort and platform-specific:

| Platform | Detection path | Coverage |
| --- | --- | --- |
| macOS | active application through AppKit/System Events | application identity when automation access is available |
| Linux Wayland | Hyprland or Sway native IPC | active window/app ID on supported compositors |
| Linux X11 | EWMH through a pure-Go X11 client | active window PID/class without spawning `xdotool` |
| Linux fallback | `xdotool` | last-resort compatibility |
| Windows | native foreground-window process lookup | active process identity |

`tg-clip status` and `tg-clipd` logs report the selected detector layer. These controls reduce exposure, but they are not end-to-end secrecy or centrally enforced policy.

## What tg-clipboard protects well

- It avoids adding another cloud account system or third-party sync service.
- It keeps traffic inside your own tailnet in normal deployments.
- It minimizes identity sprawl by reusing Tailscale's existing device/user trust.

## What tg-clipboard does not currently protect against

- A compromised or untrusted hub operator. The hub can read synced content.
- A compromised client device. Any synced clipboard is available to that device once applied locally.
- Clipboard broadcast remains non-selective; there is no per-device send or
  application authorization layer.
- End-to-end encryption from source device to destination device.
- At-rest encryption managed by tg-clipboard itself. Use OS or disk encryption if you need stronger local storage protections.
- Reliable retroactive wipe semantics after content was already synced.
- Perfect context detection. Ignore rules are best-effort and depend on what the local platform can observe.

## Important deployment caveats

### Production use

- Run `tg-clipboard` in normal tailnet mode for real usage.
- Treat every machine running `tg-clipd`, `tg-clip`, or the iOS companion as trusted with the same data you would manually copy there.

### Development mode

- `tg-clipboard -dev` is intentionally a local-development mode.
- It defaults to `localhost:8080` and does not use the tailnet identity boundary.
- If you bind it to a wider address, that is your responsibility; it is not the supported secure deployment shape.

### iOS keyboard extension

- The custom keyboard requires "Allow Full Access" to communicate with the hub.
- That requirement materially changes the trust model on the device. Only enable it if you are comfortable with that extension having networked access to clipboard-related data.

## Operator guidance

- Do not sync passwords, one-time codes, private keys, or customer secrets unless every participating device is already trusted for that class of data.
- Prefer full-disk encryption and standard device hardening on any machine that runs the hub.
- Consider lowering `--ttl` and `--max-history` if you want less clipboard retention on disk.
- If you enable privacy filters, treat them as defense-in-depth rather than a guarantee. Verify the specific rules you care about on the platforms you run.
- Use Tailscale ACLs and device hygiene as your primary access-control layer.

## Vulnerability reporting

Private reporting instructions live in [SECURITY.md](../SECURITY.md). Public issues and pull requests are not the right place for security disclosures.
