import Foundation
import Observation
import ClipHubKit
import UIKit
import ActivityKit

@Observable
final class AppViewModel {
    private let store = AppGroupStore()
    private let cache = ClipCache()
    private var client: ClipHubClient
    private var wsManager = WebSocketManager()

    var currentClip: ClipItem?
    var history: [ClipItem] = []
    var devices: [Device] = []
    var transfers: [Transfer] = []
    var receivingTransferIDs: Set<String> = []
    var connectionState: ConnectionState = .disconnected(reason: "Not started")
    var showOnboarding: Bool
    var isLoading = false
    var errorMessage: String?

    init() {
        self.showOnboarding = !store.onboardingCompleted
        self.client = ClipHubClient(baseURL: store.hubURL, sourceName: store.sourceName)
        self.history = cache.load()
        self.currentClip = store.cachedCurrentClip

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
            await refresh()
            setupWebSocket()
        }
        return reachable
    }

    // MARK: - Data

    func refresh() async {
        isLoading = true
        defer { isLoading = false }

        do {
            _ = try await client.registerDevice(
                deviceID: store.deviceID,
                name: UIDevice.current.name
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

            store.cachedCurrentClip = clip
            store.cachedRecentClips = hist
            cache.save(hist)
        } catch {
            errorMessage = error.localizedDescription
        }
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
            await refresh()
        } catch {
            errorMessage = error.localizedDescription
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
            errorMessage = error.localizedDescription
        }
    }

    func sendToHub(content: String, mimeType: String = "text/plain") async {
        do {
            let item = try await client.postClip(content: content, mimeType: mimeType)
            currentClip = item
            errorMessage = nil
        } catch {
            errorMessage = error.localizedDescription
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

    func copyCurrentToPasteboard() {
        guard let clip = currentClip else { return }
        if clip.isText, let content = clip.content {
            UIPasteboard.general.string = content
        } else if clip.mimeType == "image/png", let data = clip.data {
            UIPasteboard.general.image = UIImage(data: data)
        }
    }

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
        } catch {
            errorMessage = error.localizedDescription
        }
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
        wsManager.onUpdate = { [weak self] item in
            guard let self else { return }
            Task { @MainActor in
                self.currentClip = item
                if !self.history.contains(where: { $0.seq == item.seq }) {
                    self.history.insert(item, at: 0)
                }
                self.store.cachedCurrentClip = item
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

// Extension to allow updating the actor's URL from outside
extension ClipHubClient {
    public func updateBaseURL(_ url: URL) async {
        baseURL = url
    }
}
