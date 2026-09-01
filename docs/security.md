# Security and privacy

Tailboard is a private personal tool, not an internet-facing service. Its trust
boundary is the existing Tailscale tailnet plus the local OS accounts on each
device.

- The hub binds to the Mac's Tailscale address rather than a public interface.
- Transport confidentiality and device admission come from Tailscale.
- The hub stores bounded clipboard history in a local SQLite database.
- Mobile clients may cache recent text locally.
- Privacy filters can block configured apps, processes, or sensitive classes,
  but they are best-effort and disabled by the default personal configuration.
- macOS pasteboard entries explicitly marked concealed or transient are not
  synchronized or retained.
- System clipboard managers may independently retain synchronized text.
- Taildrop, not Tailboard, owns file transport and its security boundary.

Do not place a public route in front of the hub without adding application-level
authentication. Do not sync secrets unless every tailnet device and local user
account is trusted with them.
