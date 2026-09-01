import Foundation

/// Mirrors the hub's text-only clipboard item.
public struct ClipItem: Codable, Identifiable, Hashable, Sendable {
    public let seq: UInt64
    public let content: String
    public let hash: String
    public let source: String
    public let deviceID: String?
    public let createdAt: Date
    public let expiresAt: Date

    public var id: UInt64 { seq }

    public var preview: String {
        String(content.prefix(200)).replacingOccurrences(of: "\n", with: " ")
    }

    public var isLink: Bool {
        let trimmed = content.trimmingCharacters(in: .whitespacesAndNewlines)
        guard !trimmed.isEmpty,
              trimmed.rangeOfCharacter(from: .whitespacesAndNewlines) == nil,
              let url = URL(string: trimmed),
              let scheme = url.scheme?.lowercased(),
              scheme == "http" || scheme == "https" else { return false }
        return url.host != nil
    }

    public var kindSymbol: String { isLink ? "link" : "doc.text" }
    public var displaySummary: String { isLink ? "Link" : "Text" }

    enum CodingKeys: String, CodingKey {
        case seq, content, hash, source
        case deviceID = "device_id"
        case createdAt = "created_at"
        case expiresAt = "expires_at"
    }
}
