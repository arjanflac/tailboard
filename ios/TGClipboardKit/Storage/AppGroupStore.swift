import Foundation

public struct ControlDiagnostic: Sendable {
    public let action: String
    public let status: String
    public let detail: String
    public let updatedAt: Date
}

/// Shared configuration and cache accessible by all targets via App Group.
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
            // This personal build has a fixed MagicDNS hub. Keeping a
            // fallback here makes App Intents and extensions independent of
            // keychain availability in their short-lived processes.
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

    public var defaultTransferDeviceName: String {
        get { defaults.string(forKey: "defaultTransferDeviceName") ?? "" }
        nonmutating set {
            defaults.set(
                newValue.trimmingCharacters(in: .whitespacesAndNewlines),
                forKey: "defaultTransferDeviceName"
            )
        }
    }

    // MARK: - Control Center diagnostics

    public var lastControlDiagnostic: ControlDiagnostic? {
        guard let action = defaults.string(forKey: "lastControlAction"),
              let status = defaults.string(forKey: "lastControlStatus"),
              let updatedAt = defaults.object(forKey: "lastControlUpdatedAt") as? Date else {
            return nil
        }
        return ControlDiagnostic(
            action: action,
            status: status,
            detail: defaults.string(forKey: "lastControlDetail") ?? "",
            updatedAt: updatedAt
        )
    }

    public func recordControlDiagnostic(action: String, status: String, detail: String = "") {
        defaults.set(action, forKey: "lastControlAction")
        defaults.set(status, forKey: "lastControlStatus")
        defaults.set(detail, forKey: "lastControlDetail")
        defaults.set(Date(), forKey: "lastControlUpdatedAt")
        // Control extensions are intentionally short lived. Flush this tiny
        // diagnostic record before the process is suspended so a failed tap
        // can always be inspected from the containing app or CoreDevice.
        defaults.synchronize()
    }

    public func requestControlAction(_ action: String) {
        defaults.set(action, forKey: "pendingControlAction")
        defaults.synchronize()
    }

    public func takePendingControlAction() -> String? {
        guard let action = defaults.string(forKey: "pendingControlAction") else { return nil }
        defaults.removeObject(forKey: "pendingControlAction")
        defaults.synchronize()
        return action
    }

    public var configuredControlKinds: [String] {
        defaults.stringArray(forKey: "configuredControlKinds") ?? []
    }

    public var configuredControlsUpdatedAt: Date? {
        defaults.object(forKey: "configuredControlsUpdatedAt") as? Date
    }

    public func recordConfiguredControls(_ kinds: [String]) {
        defaults.set(kinds.sorted(), forKey: "configuredControlKinds")
        defaults.set(Date(), forKey: "configuredControlsUpdatedAt")
        defaults.synchronize()
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

    // MARK: - Cached current clip (for keyboard extension)

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

    // MARK: - Cached recent clips (for keyboard, max 10)

    public var cachedRecentClips: [ClipItem] {
        get {
            guard let data = defaults.data(forKey: "cachedRecentClips") else { return [] }
            let d = JSONDecoder(); d.dateDecodingStrategy = .tgSpringISO8601
            return (try? d.decode([ClipItem].self, from: data)) ?? []
        }
        nonmutating set {
            let e = JSONEncoder(); e.dateEncodingStrategy = .iso8601
            let trimmed = Array(newValue.prefix(10))
            defaults.set(try? e.encode(trimmed), forKey: "cachedRecentClips")
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
