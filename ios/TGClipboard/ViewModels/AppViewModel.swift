import Foundation
import Observation
import TGClipboardKit
import UIKit

/// Top-level tabs. Clipboard first: that is the app's purpose.
enum AppTab: Hashable {
    case clipboard
    case devices
    case settings
}

@Observable
final class AppViewModel {
    private let store = AppGroupStore()
    private let cache = ClipCache()
    private var client: TGClipboardClient
    private var wsManager = WebSocketManager()

    var currentClip: ClipItem?
    var history: [ClipItem] = []
    var devices: [Device] = []
    var connectionState: ConnectionState = .disconnected(reason: "Not started")
    var showOnboarding: Bool
    var isLoading = false
    var errorMessage: String?
    private(set) var privacyWipeRequested = false

    // MARK: - Navigation / presentation state

    var selectedTab: AppTab = .clipboard
    /// True while the reconfiguration sheet (onboarding with Cancel) is up.
    var isReconfiguring = false
    init() {
        let arguments = ProcessInfo.processInfo.arguments
        let shouldWipe = arguments.contains("--privacy-wipe")
        let initialStore = AppGroupStore()
        let initialCache = ClipCache()
        for argument in arguments {
            if argument.hasPrefix("--tailboard-hub="),
               let url = URL(string: String(argument.dropFirst("--tailboard-hub=".count))) {
                initialStore.hubURL = url
            } else if argument.hasPrefix("--tailboard-device-name=") {
                initialStore.sourceName = String(
                    argument.dropFirst("--tailboard-device-name=".count)
                )
            }
        }
        if shouldWipe {
            // Clear first, before any clipboard payload is decoded into the
            // view model or rendered by SwiftUI.
            initialStore.clearClipboardCache()
            initialCache.clear()
            UIPasteboard.general.items = []
        }
        self.showOnboarding = !initialStore.onboardingCompleted
        self.client = TGClipboardClient(
            baseURL: initialStore.hubURL,
            sourceName: initialStore.sourceName
        )
        self.history = shouldWipe ? [] : initialCache.load()
        self.currentClip = shouldWipe ? nil : initialStore.cachedCurrentClip
        self.privacyWipeRequested = shouldWipe

    }

    @MainActor
    func finishPrivacyWipe() async {
        guard privacyWipeRequested else { return }
        currentClip = nil
        history = []
        store.clearClipboardCache()
        cache.clear()
        UIPasteboard.general.items = []
        privacyWipeRequested = false
    }

    /// Renames this device: persists the name and re-registers with the hub
    /// so every roster shows it immediately.
    func renameDevice(to name: String) async {
        let trimmed = name.trimmingCharacters(in: .whitespacesAndNewlines)
        guard !trimmed.isEmpty, trimmed != store.sourceName else { return }
        store.sourceName = trimmed
        await client.updateSourceName(trimmed)
        await refresh()
    }

    // MARK: - Configuration

    func configureHub(url: URL, sourceName: String) async -> Bool {
        let testClient = TGClipboardClient(baseURL: url, sourceName: sourceName)
        let reachable = await testClient.probe()
        if reachable {
            store.hubURL = url
            store.sourceName = sourceName
            store.onboardingCompleted = true
            await client.updateBaseURL(url)
            wsManager.baseURL = url
            showOnboarding = false
            isReconfiguring = false
            await refresh()
            setupWebSocket()
        }
        return reachable
    }

    /// Opens onboarding as a dismissable sheet on top of a configured app.
    func beginReconfigure() {
        isReconfiguring = true
    }

    // MARK: - Data

    func refresh() async {
        isLoading = true
        defer { isLoading = false }

        do {
            _ = try await client.registerDevice(
                deviceID: store.deviceID,
                name: store.sourceName
            )
            async let clipTask = client.getCurrentClip()
            async let devicesTask = client.getDevices()
            let (clip, registeredDevices) = try await (clipTask, devicesTask)
            currentClip = clip
            devices = registeredDevices
            errorMessage = nil

            cacheCurrentClip(clip)
            if let clip { remember(clip) } else { pruneHistory() }
        } catch {
            errorMessage = UserFacingError.message(error)
        }
    }

    private func cacheCurrentClip(_ clip: ClipItem?) {
        store.cachedCurrentClip = clip
    }

    /// Removes a device from the roster (offline strays, retired hardware).
    func removeDevice(_ device: Device) async {
        do {
            try await client.removeDevice(deviceID: device.deviceID)
            devices.removeAll { $0.deviceID == device.deviceID }
        } catch {
            errorMessage = UserFacingError.message(error)
        }
    }

    func sendToHub(content: String) async {
        do {
            let item = try await client.postClip(content: content)
            currentClip = item
            remember(item)
            errorMessage = nil
        } catch {
            errorMessage = UserFacingError.message(error)
        }
    }

    /// Pushes plain text from the iPhone pasteboard to the hub.
    /// Returns true on success so callers can confirm causally.
    @discardableResult
    func pushPasteboardToHub() async -> Bool {
        do {
            if let text = UIPasteboard.general.string, !text.isEmpty {
                currentClip = try await client.postClip(content: text)
                if let currentClip { remember(currentClip) }
            } else {
                errorMessage = "iPhone clipboard is empty."
                return false
            }
            errorMessage = nil
            cacheCurrentClip(currentClip)
            return true
        } catch {
            errorMessage = UserFacingError.message(error)
            return false
        }
    }

    func setSceneActive(_ active: Bool) {
        if active {
            setupWebSocket()
            Task { await refresh() }
        } else {
            wsManager.stop()
        }
    }

    // MARK: - Pasteboard

    /// Writes the text clip to the general pasteboard.
    @discardableResult
    func copyToPasteboard(_ item: ClipItem) -> Bool {
        UIPasteboard.general.string = item.content
        return true
    }

    // MARK: - WebSocket

    private func setupWebSocket() {
        wsManager.baseURL = store.hubURL
        wsManager.deviceID = store.deviceID
        wsManager.onConnectionStateChange = { [weak self] state in
            Task { @MainActor in
                self?.connectionState = state
            }
        }
        wsManager.onUpdate = { [weak self] item in
            guard let self else { return }
            Task { @MainActor in
                self.currentClip = item
                self.remember(item)
                self.cacheCurrentClip(item)
            }
        }
        wsManager.start()
    }

    private func remember(_ item: ClipItem) {
        history = ClipCache.retained([item] + history)
        cache.save(history)
    }

    private func pruneHistory() {
        history = ClipCache.retained(history)
        cache.save(history)
    }
}

// Extensions to allow updating the actor's config from outside
extension TGClipboardClient {
    public func updateBaseURL(_ url: URL) async {
        baseURL = url
    }

    public func updateSourceName(_ name: String) async {
        sourceName = name
    }
}
