import Foundation
import Observation
import TGClipboardKit
import UIKit
import UniformTypeIdentifiers
import ActivityKit
import WidgetKit

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
    /// Transient confirmation shown when a copy came from the widget deep link.
    var copiedBannerVisible = false
    private var copiedBannerTask: Task<Void, Never>?

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
        WidgetCenter.shared.reloadAllTimelines()
        if #available(iOS 18.0, *) {
            ControlCenter.shared.reloadAllControls()
        }
        for activity in Activity<TGClipboardActivityAttributes>.activities {
            await activity.end(nil, dismissalPolicy: .immediate)
        }
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

    // MARK: - Deep links

    func handleDeepLink(_ url: URL) {
        guard url.host == "copy-current" else { return }
        selectedTab = .clipboard
        // Only confirm when a pasteboard write actually happened; otherwise
        // copyToPasteboard has already surfaced a clear error.
        if copyCurrentToPasteboard() {
            showCopiedBanner()
        }
    }

    @MainActor
    func handlePendingControlAction() async {
        guard let action = store.takePendingControlAction() else { return }
        selectedTab = .clipboard
        switch action {
        case "send":
            let succeeded = await pushPasteboardToHub()
            store.recordControlDiagnostic(
                action: "Send",
                status: succeeded ? "Succeeded" : "Failed",
                detail: succeeded ? "Clipboard sent" : (errorMessage ?? "Clipboard could not be sent")
            )
        case "receive":
            let succeeded = await receiveLatestClipboard()
            store.recordControlDiagnostic(
                action: "Receive",
                status: succeeded ? "Succeeded" : "Failed",
                detail: succeeded ? "Clipboard copied on iPhone" : (errorMessage ?? "Clipboard could not be received")
            )
        default:
            break
        }
    }

    private func showCopiedBanner() {
        copiedBannerTask?.cancel()
        copiedBannerVisible = true
        copiedBannerTask = Task { @MainActor in
            try? await Task.sleep(for: .seconds(2.5))
            guard !Task.isCancelled else { return }
            self.copiedBannerVisible = false
        }
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
            async let histTask = client.getHistory(limit: 50)
            async let devicesTask = client.getDevices()
            let (clip, hist, registeredDevices) =
                try await (clipTask, histTask, devicesTask)
            currentClip = clip
            history = hist
            devices = registeredDevices
            errorMessage = nil

            cacheCurrentClip(clip)
            store.cachedRecentClips = hist
            cache.save(hist)
        } catch {
            errorMessage = UserFacingError.message(error)
        }
    }

    /// Keeps the app-group cache (read by widget/keyboard) fresh and asks
    /// WidgetCenter to reload widget timelines so the home screen never
    /// shows a stale clip. Reloads are coalesced: a WebSocket burst triggers
    /// at most one timeline reload per minute.
    private var lastWidgetReload: Date = .distantPast

    private func cacheCurrentClip(_ clip: ClipItem?) {
        store.cachedCurrentClip = clip
        let now = Date()
        guard now.timeIntervalSince(lastWidgetReload) >= 60 else { return }
        lastWidgetReload = now
        WidgetCenter.shared.reloadTimelines(ofKind: "CurrentClipWidget")
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

    func sendToHub(content: String, mimeType: String = "text/plain") async {
        do {
            let item = try await client.postClip(content: content, mimeType: mimeType)
            currentClip = item
            errorMessage = nil
        } catch {
            errorMessage = UserFacingError.message(error)
        }
    }

    /// Pushes the iPhone pasteboard (image preferred, then text) to the hub.
    /// Returns true on success so callers can confirm causally.
    @discardableResult
    func pushPasteboardToHub() async -> Bool {
        do {
            if let image = UIPasteboard.general.image, let data = image.pngData() {
                currentClip = try await client.postClip(data: data, mimeType: "image/png")
            } else if let text = UIPasteboard.general.string, !text.isEmpty {
                currentClip = try await client.postClip(content: text)
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

    @MainActor
    private func receiveLatestClipboard() async -> Bool {
        do {
            guard let clip = try await client.getCurrentClip() else {
                errorMessage = "The shared clipboard is empty."
                return false
            }
            currentClip = clip
            cacheCurrentClip(clip)
            guard copyToPasteboard(clip) else { return false }
            showCopiedBanner()
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

    /// Writes the clip to the general pasteboard. Returns true only when a
    /// pasteboard write actually happened; unsupported binary clips surface a
    /// clear error and return false so callers never confirm a no-op.
    @discardableResult
    func copyToPasteboard(_ item: ClipItem) -> Bool {
        if item.isText, let content = item.content {
            UIPasteboard.general.string = content
            return true
        }
        if item.mimeType == "image/png", let data = item.data {
            UIPasteboard.general.setData(data, forPasteboardType: UTType.png.identifier)
            return true
        }
        errorMessage = "Only text and PNG image clips can be copied on iPhone."
        return false
    }

    @discardableResult
    func copyCurrentToPasteboard() -> Bool {
        guard let clip = currentClip else { return false }
        return copyToPasteboard(clip)
    }

    /// Live Activities never kept Tailboard connected in the background and
    /// could display a stale clip as if sync were still running. End any one
    /// created by older builds during the migration away from that UI.
    func endLegacyLiveActivities() async {
        for activity in Activity<TGClipboardActivityAttributes>.activities {
            await activity.end(nil, dismissalPolicy: .immediate)
        }
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
                if !self.history.contains(where: { $0.seq == item.seq }) {
                    self.history.insert(item, at: 0)
                }
                self.cacheCurrentClip(item)
                self.store.cachedRecentClips = self.history
            }
        }
        wsManager.start()
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
