import Foundation

/// The only connection states a user ever sees (spec §3/§4): synced,
/// connecting, offline, plus one actionable condition — Tailscale is off.
/// The `reason` payload is kept for logs/diagnostics but never reaches a
/// label; raw errors are banned from the UI.
public enum ConnectionState: Sendable, Equatable {
    case connected
    case connecting
    case disconnected(reason: String)
    case noVPN

    /// Human, state-only label. Deliberately excludes raw error text.
    public var label: String {
        switch self {
        case .connected: return "Synced"
        case .connecting: return "Connecting…"
        case .disconnected: return "Offline — reconnecting…"
        case .noVPN: return "Tailscale is off"
        }
    }

    public var isConnected: Bool {
        if case .connected = self { return true }
        return false
    }

    /// Maps a connection failure to a user-visible state. When the hub is
    /// reached over the tailnet (100.64/10 address or a ts.net name),
    /// connectivity-class failures almost always mean Tailscale is off or
    /// signed out — an actionable condition, not an error to display.
    public static func from(error: Error, hubURL: URL?) -> ConnectionState {
        guard let hubURL, looksLikeTailnetAddress(hubURL) else {
            return .disconnected(reason: error.localizedDescription)
        }
        if let urlError = error as? URLError {
            switch urlError.code {
            case .notConnectedToInternet, .networkConnectionLost,
                 .cannotFindHost, .cannotConnectToHost, .dnsLookupFailed:
                return .noVPN
            default:
                break
            }
        }
        return .disconnected(reason: error.localizedDescription)
    }

    /// True when the URL points into the tailnet: a 100.64.0.0/10 (CGNAT)
    /// address or a *.ts.net MagicDNS name.
    public static func looksLikeTailnetAddress(_ url: URL) -> Bool {
        guard let host = url.host()?.lowercased() else { return false }
        if host.hasSuffix(".ts.net") { return true }
        let octets = host.split(separator: ".")
        guard octets.count == 4,
              let first = Int(octets[0]), let second = Int(octets[1]),
              octets.allSatisfy({ Int($0) != nil }) else { return false }
        return first == 100 && (64...127).contains(second)
    }
}
