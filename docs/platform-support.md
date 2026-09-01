# Platform behavior

| Direction | Behavior |
| --- | --- |
| Mac → Pixel | Automatic Tailboard text sync, normally bounded by the 100 ms Mac poll plus network scheduling. |
| Pixel → Mac | Explicit **Send Clipboard** Quick Settings tile, app action, or text share. |
| Mac ↔ iPhone | Apple Universal Clipboard for the normal same-Apple-Account workflow. |
| Pixel ↔ iPhone | Optional Tailboard iOS foreground workflow. |
| Any photo or file | Tailscale Taildrop. |

The Go desktop engine is intentionally macOS-only. There are no Linux or
Windows targets and no standalone Tailboard hub. The mobile apps require the
same reachable Tailscale tailnet as the Mac hub.
