import Foundation
import UniformTypeIdentifiers

/// Mirrors the hub's protocol.ClipItem JSON structure.
public struct ClipItem: Codable, Identifiable, Hashable, Sendable {
    public let seq: UInt64
    public let mimeType: String
    public let content: String?
    public let data: Data?
    public let hash: String
    public let source: String
    public let deviceID: String?
    public let createdAt: Date
    public let expiresAt: Date

    public var id: UInt64 { seq }
    public var isText: Bool { mimeType.hasPrefix("text/") }

    public var rawBytes: Data {
        if isText, let content {
            return Data(content.utf8)
        }
        return data ?? Data()
    }

    /// Text clips preview their content; everything else falls back to the
    /// humanized displaySummary ("Image · 2.3 MB"). Raw "[mime, N bytes]"
    /// strings are banned from every UI surface (spec §3).
    public var preview: String {
        if isText, let content {
            let trimmed = content.prefix(200).replacingOccurrences(of: "\n", with: " ")
            return String(trimmed)
        }
        return displaySummary
    }

    // MARK: - Humanized display helpers

    /// True when the clip is a single http(s) URL with no surrounding text.
    public var isLink: Bool {
        Self.isLinkText(content)
    }

    /// Locale-aware byte count (e.g. "2.3 MB").
    public var formattedByteCount: String {
        ByteCountFormatter.string(fromByteCount: Int64(rawBytes.count), countStyle: .file)
    }

    /// SF Symbol for the clip's broad kind (text/link, image, file), so every
    /// surface pairs displaySummary with the same icon.
    public var kindSymbol: String {
        if isText { return isLink ? "link" : "doc.text" }
        if mimeType.hasPrefix("image/") { return "photo" }
        return "doc"
    }

    /// Short human-readable label for UI surfaces (keyboard, share sheet, widget).
    /// Deliberately avoids raw MIME types and byte jargon: "Text", "Link",
    /// "Image · 2.3 MB", "PDF · 812 KB".
    public var displaySummary: String {
        Self.displaySummary(mimeType: mimeType, byteCount: rawBytes.count, textContent: content)
    }

    /// Shared label logic so surfaces without a ClipItem (e.g. the share
    /// extension mid-extraction) render identical strings.
    public static func displaySummary(mimeType: String, byteCount: Int, textContent: String?) -> String {
        if mimeType.hasPrefix("text/") {
            return isLinkText(textContent) ? "Link" : "Text"
        }
        let kind: String
        if mimeType.hasPrefix("image/") {
            kind = "Image"
        } else if mimeType.hasPrefix("video/") {
            kind = "Video"
        } else if mimeType.hasPrefix("audio/") {
            kind = "Audio"
        } else if let type = UTType(mimeType: mimeType),
                  let description = type.localizedDescription {
            kind = description
        } else {
            kind = "File"
        }
        let size = ByteCountFormatter.string(fromByteCount: Int64(byteCount), countStyle: .file)
        return "\(kind) · \(size)"
    }

    private static func isLinkText(_ content: String?) -> Bool {
        guard let content else { return false }
        let trimmed = content.trimmingCharacters(in: .whitespacesAndNewlines)
        guard !trimmed.isEmpty,
              trimmed.rangeOfCharacter(from: .whitespacesAndNewlines) == nil,
              let url = URL(string: trimmed),
              let scheme = url.scheme?.lowercased(),
              scheme == "http" || scheme == "https" else { return false }
        return url.host != nil
    }

    enum CodingKeys: String, CodingKey {
        case seq, content, data, hash, source
        case deviceID = "device_id"
        case mimeType = "mime_type"
        case createdAt = "created_at"
        case expiresAt = "expires_at"
    }
}
