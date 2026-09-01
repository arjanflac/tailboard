import Foundation

/// File-based cache for offline history, stored in the App Group container.
public struct ClipCache: Sendable {
    public static let maxItems = 20
    public static let maxAge: TimeInterval = 24 * 60 * 60

    private let fileURL: URL

    public init() {
        let container = FileManager.default.containerURL(
            forSecurityApplicationGroupIdentifier: AppGroupStore.suiteName
        ) ?? FileManager.default.temporaryDirectory
        self.fileURL = container.appendingPathComponent("clip_history_cache.json")
    }

    public func save(_ items: [ClipItem]) {
        let e = JSONEncoder(); e.dateEncodingStrategy = .iso8601
        guard let data = try? e.encode(Self.retained(items)) else { return }
        try? data.write(to: fileURL, options: .atomic)
    }

    public func load() -> [ClipItem] {
        guard let data = try? Data(contentsOf: fileURL) else { return [] }
        let d = JSONDecoder(); d.dateDecodingStrategy = .tgSpringISO8601
        let decoded = (try? d.decode([ClipItem].self, from: data)) ?? []
        let retained = Self.retained(decoded)
        if retained != decoded { save(retained) }
        return retained
    }

    /// Returns newest-first, de-duplicated local history with fixed personal
    /// retention. The Mac relay deliberately owns no history of its own.
    public static func retained(_ items: [ClipItem], now: Date = Date()) -> [ClipItem] {
        let cutoff = now.addingTimeInterval(-maxAge)
        var seen = Set<UInt64>()
        return Array(items
            .sorted { $0.seq > $1.seq }
            .filter { item in
                item.createdAt >= cutoff && seen.insert(item.seq).inserted
            }
            .prefix(maxItems))
    }

    /// Atomically replaces the offline history with an empty array without
    /// reading the previous file.
    public func clear() {
        save([])
    }
}
