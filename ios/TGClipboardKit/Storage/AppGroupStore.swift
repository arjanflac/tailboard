import Foundation

/// Local configuration and cache retained in the historical app-group suite.
public struct AppGroupStore: @unchecked Sendable {
    public static let suiteName = "group.com.arjanflac.tgclipboard"

    private let defaults: UserDefaults

    public init() {
        self.defaults = UserDefaults(suiteName: Self.suiteName) ?? .standard
    }

    // MARK: - Hub URL

    public var hubURL: URL? {
        get {
            if let data = SharedKeychain.read("hubURL"),
               let value = String(data: data, encoding: .utf8),
               let url = URL(string: value) {
                return url
            }
            // One-time migration from older app-group defaults.
            if let legacy = defaults.url(forKey: "hubURL") {
                SharedKeychain.write(Data(legacy.absoluteString.utf8), account: "hubURL")
                defaults.removeObject(forKey: "hubURL")
                return legacy
            }
            // This personal build has a fixed MagicDNS hub fallback.
            return URL(string: "http://tailboard-hub:9437")
        }
        nonmutating set {
            SharedKeychain.write(newValue.map { Data($0.absoluteString.utf8) }, account: "hubURL")
            defaults.removeObject(forKey: "hubURL")
        }
    }

    // MARK: - Source name

    public var sourceName: String {
        get {
            let value = defaults.string(forKey: "sourceName") ?? "iphone"
            let trimmed = value.trimmingCharacters(in: .whitespacesAndNewlines)
            return trimmed.isEmpty ? "iphone" : trimmed.lowercased()
        }
        nonmutating set {
            let trimmed = newValue.trimmingCharacters(in: .whitespacesAndNewlines)
            defaults.set(trimmed.isEmpty ? "iphone" : trimmed.lowercased(), forKey: "sourceName")
        }
    }

    public var deviceID: String {
        if let existing = defaults.string(forKey: "deviceID") {
            return existing
        }
        let generated = UUID().uuidString.lowercased()
        defaults.set(generated, forKey: "deviceID")
        return generated
    }

    // MARK: - Onboarding

    public var onboardingCompleted: Bool {
        get { defaults.bool(forKey: "onboardingCompleted") }
        nonmutating set { defaults.set(newValue, forKey: "onboardingCompleted") }
    }

    // MARK: - Last seq (for WebSocket reconnect catch-up)

    public var lastSeq: UInt64 {
        get { UInt64(defaults.integer(forKey: "lastSeq")) }
        nonmutating set { defaults.set(Int(newValue), forKey: "lastSeq") }
    }

    // MARK: - Cached current clip

    public var cachedCurrentClip: ClipItem? {
        get {
            guard let data = defaults.data(forKey: "cachedCurrentClip") else { return nil }
            let d = JSONDecoder(); d.dateDecodingStrategy = .tgSpringISO8601
            return try? d.decode(ClipItem.self, from: data)
        }
        nonmutating set {
            let e = JSONEncoder(); e.dateEncodingStrategy = .iso8601
            defaults.set(try? e.encode(newValue), forKey: "cachedCurrentClip")
        }
    }

    /// Removes clipboard payloads while preserving connection, device, and
    /// onboarding settings. This method never decodes or logs the old values.
    public func clearClipboardCache() {
        defaults.removeObject(forKey: "cachedCurrentClip")
        defaults.removeObject(forKey: "cachedRecentClips")
        defaults.set(0, forKey: "lastSeq")
        defaults.synchronize()
    }
}
