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
