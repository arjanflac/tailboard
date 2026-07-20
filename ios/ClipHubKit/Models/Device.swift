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
}

public struct TransferFile: Codable, Hashable, Sendable {
    public let name: String
    public let size: Int64
    public let mime: String
    public let sha256: String
    public let uploaded: Int64
}

public struct Transfer: Codable, Identifiable, Hashable, Sendable {
    public let transferID: String
    public let fromDevice: String
    public let toDevice: String
    public let files: [TransferFile]
    public let note: String?
    public let state: String
    public let createdAt: Date
    public let expiresAt: Date

    public var id: String { transferID }

    enum CodingKeys: String, CodingKey {
        case files, note, state
        case transferID = "transfer_id"
        case fromDevice = "from_device"
        case toDevice = "to_device"
        case createdAt = "created_at"
        case expiresAt = "expires_at"
    }
}

public struct CreateTransferResponse: Codable, Sendable {
    public let transfer: Transfer
    public let uploadURLs: [String]

    enum CodingKeys: String, CodingKey {
        case transfer
        case uploadURLs = "upload_urls"
    }
}
