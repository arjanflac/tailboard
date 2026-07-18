import Foundation

public struct WSMessage: Codable, Sendable {
    public let type: String
    public let item: ClipItem?
    public let transfer: Transfer?
}

public struct HubStatus: Codable, Sendable {
    public let uptime: String
    public let seq: UInt64
    public let subscribers: Int
}
