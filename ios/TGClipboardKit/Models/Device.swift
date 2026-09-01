import Foundation

public struct Device: Codable, Identifiable, Hashable, Sendable {
    public let deviceID: String
    public let name: String
    public let platform: String
    public let capabilities: [String]
    public let publicKey: String?
    public let online: Bool
    public let lastSeen: Date

    public var id: String { deviceID }

    /// SF Symbol for the device platform. Mirrors the main app's DevicesView
    /// mapping so every surface shows the same icon for the same platform.
    public var platformSymbol: String {
        switch platform {
        case "ios": return "iphone"
        case "darwin": return "laptopcomputer"
        case "windows": return "desktopcomputer"
        case "linux": return "terminal"
        default: return "display"
        }
    }

    /// Plain-language reachability label for pickers and lists.
    public var statusLabel: String { online ? "Online" : "Offline" }

    /// Stable hue (0..<1) derived from the device ID so every surface — iOS
    /// grid, macOS popover, share sheet — paints the same friendly avatar
    /// color for the same device. Uses djb2, not hashValue, because Swift's
    /// hashValue is seeded per-launch.
    public var avatarHue: Double {
        var hash: UInt64 = 5381
        for byte in deviceID.utf8 { hash = hash &* 33 &+ UInt64(byte) }
        return Double(hash % 360) / 360.0
    }

    /// Plain-language platform name ("Mac", "iOS", …) so user-facing lists
    /// never show raw platform identifiers like "darwin".
    public var platformLabel: String {
        switch platform {
        case "ios": return "iOS"
        case "darwin": return "Mac"
        case "windows": return "Windows"
        case "linux": return "Linux"
        default: return platform.capitalized
        }
    }

    enum CodingKeys: String, CodingKey {
        case name, platform, capabilities, online
        case deviceID = "device_id"
        case publicKey = "public_key"
        case lastSeen = "last_seen"
    }

    /// The hub omits `capabilities` when empty (Go `omitempty`) — e.g. in
    /// the register response. A missing list must decode as [], not fail:
    /// this decode aborting silently blanked the whole app, because
    /// registerDevice is the first call in refresh().
    public init(from decoder: Decoder) throws {
        let container = try decoder.container(keyedBy: CodingKeys.self)
        deviceID = try container.decode(String.self, forKey: .deviceID)
        name = try container.decode(String.self, forKey: .name)
        platform = try container.decode(String.self, forKey: .platform)
        capabilities = try container.decodeIfPresent([String].self, forKey: .capabilities) ?? []
        publicKey = try container.decodeIfPresent(String.self, forKey: .publicKey)
        online = try container.decodeIfPresent(Bool.self, forKey: .online) ?? false
        lastSeen = try container.decode(Date.self, forKey: .lastSeen)
    }
}
