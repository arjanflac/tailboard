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
    private(set) var privacyWipeRequested = false

    // MARK: - Navigation / presentation state

    var selectedTab: AppTab = .clipboard
    /// True while the reconfiguration sheet (onboarding with Cancel) is up.
    var isReconfiguring = false
    /// Transient confirmation shown when a copy came from the widget deep link.
    var copiedBannerVisible = false
    private var copiedBannerTask: Task<Void, Never>?
    var receivedBannerMessage: String?
    private var receivedBannerTask: Task<Void, Never>?
    private var sceneIsActive = false

    // MARK: - Live Activity

    private(set) var liveActivityRunning = false

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
            } else if argument.hasPrefix("--tailboard-default-target=") {
                initialStore.defaultTransferDeviceName = String(
                    argument.dropFirst("--tailboard-default-target=".count)
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
            async let transfersTask = client.getTransfers(deviceID: store.deviceID)

            let (clip, hist, registeredDevices, currentTransfers) =
                try await (clipTask, histTask, devicesTask, transfersTask)
            currentClip = clip
            history = hist
            devices = registeredDevices
            transfers = currentTransfers
            errorMessage = nil

            autoAcceptNextMacTransfer()

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
        guard !receivingTransferIDs.contains(transfer.transferID) else { return }
        receivingTransferIDs.insert(transfer.transferID)
        defer {
            receivingTransferIDs.remove(transfer.transferID)
            autoAcceptNextMacTransfer()
        }
        var acceptedTransferID: String?
        do {
            let accepted: Transfer
            if transfer.state == "accepted" || transfer.state == "transferring" {
                accepted = transfer
            } else {
                accepted = try await client.transferAction(
                    deviceID: store.deviceID,
                    transferID: transfer.transferID,
                    action: "accept"
                )
            }
            acceptedTransferID = accepted.transferID
            let documents = FileManager.default.urls(
                for: .documentDirectory,
                in: .userDomainMask
            )[0]
            try FileManager.default.createDirectory(
                at: documents,
                withIntermediateDirectories: true
            )
            var savedNames: [String] = []
            for (index, file) in accepted.files.enumerated() {
                let name = URL(fileURLWithPath: file.name).lastPathComponent
                guard !name.isEmpty, name != ".", name != ".." else {
                    throw CocoaError(.fileWriteInvalidFileName)
                }
                if try matchingDocument(in: documents, transferFile: file) != nil {
                    continue
                }
                let finalURL = availableDocumentURL(in: documents, preferredName: name)
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
                savedNames.append(finalURL.lastPathComponent)
            }
            _ = try await client.transferAction(
                deviceID: store.deviceID,
                transferID: accepted.transferID,
                action: "complete"
            )
            let receiveMessage = savedNames.isEmpty
                ? "Already saved in Files → Tailboard"
                : "Saved to Files → Tailboard"
            await MainActor.run {
                notifier.notificationOccurred(.success)
                showReceivedBanner(receiveMessage)
            }
            await refresh()
        } catch {
            if let acceptedTransferID {
                _ = try? await client.transferAction(
                    deviceID: store.deviceID,
                    transferID: acceptedTransferID,
                    action: "cancel"
                )
            }
            await MainActor.run { notifier.notificationOccurred(.error) }
            errorMessage = UserFacingError.message(error)
            await refresh()
        }
    }

    private func matchingDocument(in directory: URL, transferFile: TransferFile) throws -> URL? {
        let contents = try FileManager.default.contentsOfDirectory(
            at: directory,
            includingPropertiesForKeys: [.fileSizeKey, .isRegularFileKey],
            options: [.skipsHiddenFiles]
        )
        for candidate in contents {
            let values = try candidate.resourceValues(forKeys: [.fileSizeKey, .isRegularFileKey])
            guard values.isRegularFile == true,
                  Int64(values.fileSize ?? -1) == transferFile.size else { continue }
            if try ClipHash.sha256Hex(fileURL: candidate)
                .caseInsensitiveCompare(transferFile.sha256) == .orderedSame {
                return candidate
            }
        }
        return nil
    }

    private func availableDocumentURL(in directory: URL, preferredName: String) -> URL {
        let original = URL(fileURLWithPath: preferredName)
        let stem = original.deletingPathExtension().lastPathComponent
        let fileExtension = original.pathExtension
        for number in 1..<10_000 {
            let candidateName: String
            if number == 1 {
                candidateName = preferredName
            } else if fileExtension.isEmpty {
                candidateName = "\(stem) (\(number))"
            } else {
                candidateName = "\(stem) (\(number)).\(fileExtension)"
            }
            let candidate = directory.appendingPathComponent(candidateName)
            if !FileManager.default.fileExists(atPath: candidate.path) {
                return candidate
            }
        }
        return directory.appendingPathComponent("\(UUID().uuidString)-\(preferredName)")
    }

    private func autoAcceptNextMacTransfer() {
        guard sceneIsActive, receivingTransferIDs.isEmpty else { return }
        let macDeviceIDs = Set(
            devices
                .filter { $0.platform.caseInsensitiveCompare("darwin") == .orderedSame }
                .map(\.deviceID)
        )
        let candidates = transfers.filter {
            $0.toDevice == store.deviceID
                && macDeviceIDs.contains($0.fromDevice)
                && ($0.state == "offered" || $0.state == "accepted" || $0.state == "transferring")
        }
        guard let next = candidates.max(by: { $0.createdAt < $1.createdAt }) else { return }
        Task { @MainActor [weak self] in
            await self?.accept(next)
        }
    }

    @MainActor
    private func showReceivedBanner(_ message: String) {
        receivedBannerTask?.cancel()
        receivedBannerMessage = message
        receivedBannerTask = Task { @MainActor in
            try? await Task.sleep(for: .seconds(3))
            guard !Task.isCancelled else { return }
            self.receivedBannerMessage = nil
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
        sceneIsActive = active
        if active {
            liveActivityRunning = !Activity<TGClipboardActivityAttributes>.activities.isEmpty
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
        let state = TGClipboardActivityAttributes.ContentState(
            preview: clip.preview,
            source: clip.source,
            updatedAt: clip.createdAt
        )
        do {
            _ = try Activity.request(
                attributes: TGClipboardActivityAttributes(),
                content: ActivityContent(state: state, staleDate: clip.expiresAt),
                pushType: nil
            )
            liveActivityRunning = true
        } catch {
            errorMessage = UserFacingError.message(error)
        }
    }

    func stopLiveActivity() async {
        for activity in Activity<TGClipboardActivityAttributes>.activities {
            await activity.end(nil, dismissalPolicy: .immediate)
        }
        liveActivityRunning = false
    }

    private func updateLiveActivities(with clip: ClipItem) {
        let state = TGClipboardActivityAttributes.ContentState(
            preview: clip.preview,
            source: clip.source,
            updatedAt: clip.createdAt
        )
        Task {
            for activity in Activity<TGClipboardActivityAttributes>.activities {
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
                self.autoAcceptNextMacTransfer()
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
