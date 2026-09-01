# Roadmap

See also: [Architecture](architecture.md), [Known Limitations](limitations.md),
and [Public Release Checklist](public-release-checklist.md).

## Shipped

- Cross-platform Go clipboard engine for plain text, HTML, and PNG.
- Persistent hub with bounded history, health endpoints, metrics, and a stable
  device registry.
- Embedded Mac hub and lightweight `SMAppService` background engine.
- Native Mac menu-bar app.
- Android app with automatic receive, Quick Settings send, and text sharing.
- iOS app, keyboard, text/link sharing, widget, Shortcuts, and Control Center
  controls with honest foreground-only behavior.
- Local privacy filters and explicit clipboard/history clearing.

## Next

- Exercise the daily Mac/Pixel/iPhone workflow and fix only observed clipboard
  reliability or UX problems.
- Finish repeatable Developer ID notarization and TestFlight distribution.
- Replace personal setup defaults with a clear onboarding flow for another
  tailnet.
- Decide whether to maintain Linux/Windows CLI support as part of Tailboard or
  leave those platforms entirely to upstream.
- Prepare a focused upstream contribution for improvements that are generic to
  tg-clipboard rather than Tailboard-specific product work.

## Deliberate boundaries

- Clipboard state remains hub-mediated, broadcast, and last-write-wins.
- iOS remains user-initiated.
- Photos and files remain the responsibility of Tailscale.
- Tailboard does not add another account, pairing protocol, or VPN stack.
