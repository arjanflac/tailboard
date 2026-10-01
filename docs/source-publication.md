# Source publication review

Reviewed September 30, 2026. Tailboard is shared as MIT-licensed source for users
to build themselves. No maintainer signing service, binary releases, or support
commitment.

## Checks

- Full Git history and all 26 completed GitHub Actions runs: no known secrets
  found. No personal device addresses, local paths, or signing identities in
  Actions logs. No uploaded build artifacts or GitHub Releases.
- Go: tests, race checks, vet, Staticcheck, dead-code analysis, and vulnerability
  scanning. No unreachable functions or known dependency vulnerabilities found.
- Android: runtime dependencies checked against OSV; no known vulnerabilities
  found across the 10 resolved packages. Unit tests, lint, and builds checked.
- Clipboard text is not written to app storage or logs. Android backup is
  disabled; only settings and the last receipt ID are saved.
- MIT attribution and dependency notices are retained. Git author attribution
  remains part of the public source history.

## Footprint

An idle sample with one connected Pixel, on the maintainer's devices:

| Component | Observed footprint |
| --- | --- |
| Mac engine | 5.6–9.8 MiB resident memory; 0–0.2% CPU over 24 seconds |
| Android app | About 38 MiB proportional memory |
| Build sizes | About 8.8 MiB engine; 4 MiB debug APK |

These are snapshots, not memory limits or battery-life benchmarks. There is no
embedded Tailscale node, database, analytics, or clipboard history. Android
clients share connection pools and reconnect with backoff.

## Trust boundary

Tailscale access rules decide who can reach port 9437. Allowed devices can read,
replace, or clear the clipboard; there is no additional app login. Keep the
service private and restrict it to trusted devices.

Automatic discovery excludes LAN interfaces and refuses ambiguous CGNAT VPN
matches. Browser Origin/Fetch Metadata requests and public DNS Host headers are
rejected; clipboard responses cannot be cached. Payloads, metadata, stream counts,
and network waits are bounded. Clients do not follow
HTTP redirects or use system proxies. These checks reduce specific risks;
they do not make an untrusted tailnet safe.

The optional Mac app is locally development-signed. Users can instead install
the engine-only login agent without an Apple account. Build instructions and
troubleshooting are in [building.md](building.md); the README keeps the setup short.
