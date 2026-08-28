import Foundation
import AppKit
import Observation
import UserNotifications
import TGClipboardKit

/// What the menu bar icon communicates at a glance.
enum EngineStatus: Equatable {
    case synced
    case offline
    case noTailscale
    case paused
    case engineOff

    var symbolName: String {
        switch self {
        case .synced: return "doc.on.clipboard"
        case .offline: return "doc.on.clipboard"
        case .noTailscale: return "network.slash"
        case .paused: return "pause.circle"
        case .engineOff: return "exclamationmark.triangle"
        }
    }
}

@MainActor
@Observable
final class MenuBarViewModel: NSObject {
    private let client = ControlClient()
    private var pollTask: Task<Void, Never>?
    private var notifiedOfferIDs: Set<String> = []

    var state: ControlState?
    var errorMessage: String?
    /// Files staged by a drop on the menu bar icon, waiting for a device pick.
    var stagedFileURLs: [URL] = []
    var sendingDeviceIDs: Set<String> = []
    /// Device that just received a successful send (for the ✓ flash).
    var justSentDeviceID: String?

    var status: EngineStatus {
        guard let state else { return .engineOff }
        if state.paused { return .paused }
        switch state.connection {
        case "no-tailscale": return .noTailscale
        case "offline": return .offline
        default: return .synced
        }
    }

    var deviceID: String { state?.deviceID ?? "" }

    /// Other devices, online first — never this Mac.
    var otherDevices: [Device] {
        (state?.devices ?? [])
            .filter { $0.deviceID != deviceID }
            .sorted { ($0.online ? 0 : 1, $0.name) < ($1.online ? 0 : 1, $1.name) }
    }

    var incomingOffers: [Transfer] {
        (state?.transfers ?? []).filter { $0.state == "offered" && $0.toDevice == deviceID }
    }

    var activeTransfers: [Transfer] {
        (state?.transfers ?? []).filter {
            ($0.state == "accepted" || $0.state == "transferring") ||
            ($0.state == "complete" && $0.createdAt > .now.addingTimeInterval(-300))
        }
    }

    var statusLine: String {
        switch status {
        case .engineOff: return "Tailboard Engine isn't running"
        case .paused: return "Paused"
        case .offline: return "Offline — reconnecting…"
        case .noTailscale: return "Tailscale is off"
        case .synced:
            let count = otherDevices.filter(\.online).count
            switch count {
            case 0: return "Synced · no devices online"
            case 1: return "Synced · 1 device"
            default: return "Synced · \(count) devices"
            }
        }
    }

    func openTailscale() {
        let tailscaleApp = URL(fileURLWithPath: "/Applications/Tailscale.app")
        if FileManager.default.fileExists(atPath: tailscaleApp.path) {
            NSWorkspace.shared.openApplication(at: tailscaleApp, configuration: .init())
        } else if let url = URL(string: "https://tailscale.com/download/macos") {
            NSWorkspace.shared.open(url)
        }
    }

    // MARK: - Lifecycle

    func start() {
        guard pollTask == nil else { return }
        UNUserNotificationCenter.current().delegate = self
        registerNotificationCategories()
        Task {
            _ = try? await UNUserNotificationCenter.current()
                .requestAuthorization(options: [.alert, .sound])
        }
        pollTask = Task { [weak self] in
            while !Task.isCancelled {
                await self?.refresh()
                try? await Task.sleep(for: .seconds(2))
            }
        }
    }

    func refresh() async {
        do {
            let newState = try await client.state()
            state = newState
            notifyNewOffers(in: newState)
        } catch {
            state = nil
        }
    }

    // MARK: - Actions

    func togglePaused() {
        guard let state else { return }
        Task {
            try? await client.setPaused(!state.paused)
            await refresh()
        }
    }

    func accept(_ transfer: Transfer) {
        Task {
            do {
                try await client.transferAction(id: transfer.transferID, action: "accept")
            } catch {
                errorMessage = UserFacingError.message(error)
            }
            await refresh()
        }
    }

    func decline(_ transfer: Transfer) {
        Task {
            try? await client.transferAction(id: transfer.transferID, action: "decline")
            await refresh()
        }
    }

    func revealDownloads() {
        Task { try? await client.revealDownloads() }
    }

    func copyCurrentClip() {
        guard let clip = state?.clip, clip.isText, let content = clip.content else { return }
        NSPasteboard.general.clearContents()
        NSPasteboard.general.setString(content, forType: .string)
    }

    func send(fileURLs: [URL], to device: Device) {
        guard !fileURLs.isEmpty else { return }
        sendingDeviceIDs.insert(device.deviceID)
        Task {
            defer { sendingDeviceIDs.remove(device.deviceID) }
            do {
                try await client.send(fileURLs: fileURLs, to: device.deviceID)
                flashSent(device.deviceID)
            } catch {
                errorMessage = (error as? ControlError)?.errorDescription
                    ?? UserFacingError.message(error)
            }
            await refresh()
        }
    }

    /// Sends files staged by a drop on the menu bar icon.
    func sendStaged(to device: Device) {
        let files = stagedFileURLs
        stagedFileURLs = []
        send(fileURLs: files, to: device)
    }

    func pickAndSendFiles(to device: Device) {
        let panel = NSOpenPanel()
        panel.canChooseFiles = true
        panel.canChooseDirectories = false
        panel.allowsMultipleSelection = true
        panel.message = "Send to \(device.name)"
        panel.prompt = "Send"
        NSApp.activate(ignoringOtherApps: true)
        if panel.runModal() == .OK {
            send(fileURLs: panel.urls, to: device)
        }
    }

    /// Starts the tg-clipd launch agent via tg-clip (best effort).
    func startEngine() {
        Task.detached {
            let candidates = [
                "/opt/homebrew/bin/tg-clip", "/usr/local/bin/tg-clip",
                NSString("~/go/bin/tg-clip").expandingTildeInPath,
            ]
            guard let bin = candidates.first(where: { FileManager.default.isExecutableFile(atPath: $0) }) else { return }
            let process = Process()
            process.executableURL = URL(fileURLWithPath: bin)
            process.arguments = ["service", "install"]
            try? process.run()
            process.waitUntilExit()
        }
        Task {
            try? await Task.sleep(for: .seconds(2))
            await refresh()
        }
    }

    private func flashSent(_ deviceID: String) {
        justSentDeviceID = deviceID
        Task {
            try? await Task.sleep(for: .seconds(2))
            if justSentDeviceID == deviceID { justSentDeviceID = nil }
        }
    }

    // MARK: - Notifications (replace osascript: actionable Accept/Decline)

    private static let offerCategory = "TRANSFER_OFFER"

    private func registerNotificationCategories() {
        let accept = UNNotificationAction(identifier: "ACCEPT", title: "Accept", options: [])
        let decline = UNNotificationAction(identifier: "DECLINE", title: "Decline", options: [.destructive])
        let category = UNNotificationCategory(
            identifier: Self.offerCategory,
            actions: [accept, decline],
            intentIdentifiers: []
        )
        UNUserNotificationCenter.current().setNotificationCategories([category])
    }

    private func notifyNewOffers(in state: ControlState) {
        for offer in incomingOffers where !notifiedOfferIDs.contains(offer.transferID) {
            notifiedOfferIDs.insert(offer.transferID)
            let from = (state.devices ?? []).first { $0.deviceID == offer.fromDevice }?.name ?? "Unknown device"
            let content = UNMutableNotificationContent()
            content.title = "\(from) wants to send you \(Self.filesPhrase(offer))"
            content.body = offer.files.map(\.name).joined(separator: ", ")
            content.categoryIdentifier = Self.offerCategory
            content.userInfo = ["transfer_id": offer.transferID]
            content.sound = .default
            let request = UNNotificationRequest(
                identifier: "offer-\(offer.transferID)", content: content, trigger: nil
            )
            UNUserNotificationCenter.current().add(request)
        }
    }

    static func filesPhrase(_ transfer: Transfer) -> String {
        let count = transfer.files.count
        let total = transfer.files.reduce(Int64(0)) { $0 + $1.size }
        let size = ByteCountFormatter.string(fromByteCount: total, countStyle: .file)
        return count == 1 ? "a file (\(size))" : "\(count) files (\(size))"
    }
}

extension MenuBarViewModel: UNUserNotificationCenterDelegate {
    nonisolated func userNotificationCenter(
        _ center: UNUserNotificationCenter,
        didReceive response: UNNotificationResponse,
        withCompletionHandler completionHandler: @escaping () -> Void
    ) {
        let userInfo = response.notification.request.content.userInfo
        let action = response.actionIdentifier
        guard let transferID = userInfo["transfer_id"] as? String else {
            completionHandler()
            return
        }
        Task { @MainActor in
            if let transfer = (self.state?.transfers ?? []).first(where: { $0.transferID == transferID }) {
                if action == "ACCEPT" { self.accept(transfer) }
                if action == "DECLINE" { self.decline(transfer) }
            }
            completionHandler()
        }
    }

    nonisolated func userNotificationCenter(
        _ center: UNUserNotificationCenter,
        willPresent notification: UNNotification,
        withCompletionHandler completionHandler: @escaping (UNNotificationPresentationOptions) -> Void
    ) {
        completionHandler([.banner, .sound])
    }
}
