import Foundation
import Observation
import ClipHubKit
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
    private var client: ClipHubClient
    private var wsManager = WebSocketManager()
    private let notifier = UINotificationFeedbackGenerator()

    var currentClip: ClipItem?
    var history: [ClipItem] = []
    var devices: [Device] = []
    var transfers: [Transfer] = []
    var receivingTransferIDs: Set<String> = []
    var connectionState: ConnectionState = .disconnected(reason: "Not started")
    var showOnboarding: Bool
    var isLoading = false
    var errorMessage: String?

    // MARK: - Navigation / presentation state

    var selectedTab: AppTab = .clipboard
    /// True while the reconfiguration sheet (onboarding with Cancel) is up.
    var isReconfiguring = false
    /// Transient confirmation shown when a copy came from the widget deep link.
    var copiedBannerVisible = false
    private var copiedBannerTask: Task<Void, Never>?

    // MARK: - Live Activity

    private(set) var liveActivityRunning = false

    init() {
        self.showOnboarding = !store.onboardingCompleted
        // iOS censors UIDevice.name to "iPhone" for third-party apps, so the
        // user-chosen name in the store is the identity; upgrade the legacy
        // lowercase default once.
        if store.sourceName == "iphone" {
            store.sourceName = UIDevice.current.name
        }
        self.client = ClipHubClient(baseURL: store.hubURL, sourceName: store.sourceName)
        self.history = cache.load()
        self.currentClip = store.cachedCurrentClip

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
        let testClient = ClipHubClient(baseURL: url, sourceName: sourceName)
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
            async let transfersTask = client.getTransfers(deviceID: store.deviceID)

            let (clip, hist, registeredDevices, currentTransfers) =
                try await (clipTask, histTask, devicesTask, transfersTask)
            currentClip = clip
            history = hist
            devices = registeredDevices
            transfers = currentTransfers
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

    func accept(_ transfer: Transfer) async {
        receivingTransferIDs.insert(transfer.transferID)
        defer { receivingTransferIDs.remove(transfer.transferID) }
        do {
            let accepted = try await client.transferAction(
                deviceID: store.deviceID,
                transferID: transfer.transferID,
                action: "accept"
            )
            let documents = FileManager.default.urls(
                for: .documentDirectory,
                in: .userDomainMask
            )[0]
            for (index, file) in accepted.files.enumerated() {
                let name = URL(fileURLWithPath: file.name).lastPathComponent
                guard !name.isEmpty, name != ".", name != ".." else {
                    throw CocoaError(.fileWriteInvalidFileName)
                }
                let finalURL = documents.appendingPathComponent(name)
                guard !FileManager.default.fileExists(atPath: finalURL.path) else {
                    throw CocoaError(.fileWriteFileExists)
                }
                let partialURL = documents.appendingPathComponent(".\(UUID().uuidString).part")
                defer { try? FileManager.default.removeItem(at: partialURL) }
                try await client.downloadTransferFile(
                    deviceID: store.deviceID,
                    transferID: accepted.transferID,
                    index: index,
                    destination: partialURL
                )
                guard try ClipHash.sha256Hex(fileURL: partialURL)
                    .caseInsensitiveCompare(file.sha256) == .orderedSame else {
                    throw CocoaError(.fileReadCorruptFile)
                }
                try FileManager.default.moveItem(at: partialURL, to: finalURL)
            }
            _ = try await client.transferAction(
                deviceID: store.deviceID,
                transferID: accepted.transferID,
                action: "complete"
            )
            await MainActor.run { notifier.notificationOccurred(.success) }
            await refresh()
        } catch {
            await MainActor.run { notifier.notificationOccurred(.error) }
            errorMessage = UserFacingError.message(error)
        }
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

    func decline(_ transfer: Transfer) async {
        do {
            _ = try await client.transferAction(
                deviceID: store.deviceID,
                transferID: transfer.transferID,
                action: "decline"
            )
            await refresh()
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

    func setSceneActive(_ active: Bool) {
        if active {
            liveActivityRunning = !Activity<ClipHubActivityAttributes>.activities.isEmpty
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

    // MARK: - Live Activity

    func startLiveActivity() async {
        guard ActivityAuthorizationInfo().areActivitiesEnabled,
              let clip = currentClip else { return }
        let state = ClipHubActivityAttributes.ContentState(
            preview: clip.preview,
            source: clip.source,
            updatedAt: clip.createdAt
        )
        do {
            _ = try Activity.request(
                attributes: ClipHubActivityAttributes(),
                content: ActivityContent(state: state, staleDate: clip.expiresAt),
                pushType: nil
            )
            liveActivityRunning = true
        } catch {
            errorMessage = UserFacingError.message(error)
        }
    }

    func stopLiveActivity() async {
        for activity in Activity<ClipHubActivityAttributes>.activities {
            await activity.end(nil, dismissalPolicy: .immediate)
        }
        liveActivityRunning = false
    }

    private func updateLiveActivities(with clip: ClipItem) {
        let state = ClipHubActivityAttributes.ContentState(
            preview: clip.preview,
            source: clip.source,
            updatedAt: clip.createdAt
        )
        Task {
            for activity in Activity<ClipHubActivityAttributes>.activities {
                await activity.update(
                    ActivityContent(state: state, staleDate: clip.expiresAt)
                )
            }
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
                self.updateLiveActivities(with: item)
            }
        }
        wsManager.onTransfer = { [weak self] transfer in
            guard let self else { return }
            Task { @MainActor in
                if let index = self.transfers.firstIndex(where: { $0.id == transfer.id }) {
                    self.transfers[index] = transfer
                } else {
                    self.transfers.insert(transfer, at: 0)
                }
            }
        }
        wsManager.start()
    }
}

// Extensions to allow updating the actor's config from outside
extension ClipHubClient {
    public func updateBaseURL(_ url: URL) async {
        baseURL = url
    }

    public func updateSourceName(_ name: String) async {
        sourceName = name
    }
}
