import Foundation

/// Shared configuration and cache accessible by all targets via App Group.
public struct AppGroupStore: @unchecked Sendable {
    public static let suiteName = "group.com.thalys.cliphub"

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
            return nil
        }
        nonmutating set {
            SharedKeychain.write(newValue.map { Data($0.absoluteString.utf8) }, account: "hubURL")
            defaults.removeObject(forKey: "hubURL")
        }
    }

    // MARK: - Source name

    public var sourceName: String {
        get { defaults.string(forKey: "sourceName") ?? "iphone" }
        nonmutating set { defaults.set(newValue, forKey: "sourceName") }
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

    // MARK: - Cached current clip (for keyboard extension)

    public var cachedCurrentClip: ClipItem? {
        get {
            guard let data = defaults.data(forKey: "cachedCurrentClip") else { return nil }
            let d = JSONDecoder(); d.dateDecodingStrategy = .iso8601
            return try? d.decode(ClipItem.self, from: data)
        }
        nonmutating set {
            let e = JSONEncoder(); e.dateEncodingStrategy = .iso8601
            defaults.set(try? e.encode(newValue), forKey: "cachedCurrentClip")
        }
    }

    // MARK: - Cached recent clips (for keyboard, max 10)

    public var cachedRecentClips: [ClipItem] {
        get {
            guard let data = defaults.data(forKey: "cachedRecentClips") else { return [] }
            let d = JSONDecoder(); d.dateDecodingStrategy = .iso8601
            return (try? d.decode([ClipItem].self, from: data)) ?? []
        }
        nonmutating set {
            let e = JSONEncoder(); e.dateEncodingStrategy = .iso8601
            let trimmed = Array(newValue.prefix(10))
            defaults.set(try? e.encode(trimmed), forKey: "cachedRecentClips")
        }
    }
}
