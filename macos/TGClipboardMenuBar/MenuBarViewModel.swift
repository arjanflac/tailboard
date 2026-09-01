import AppKit
import Foundation
import Observation
import TGClipboardKit

enum EngineStatus: Equatable {
    case synced
    case offline
    case noTailscale
    case paused
    case engineOff
}

@MainActor
@Observable
final class MenuBarViewModel {
    private let client = ControlClient()
    private var pollTask: Task<Void, Never>?

    var state: ControlState?
    var errorMessage: String?

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

    var otherDevices: [Device] {
        (state?.devices ?? [])
            .filter { $0.deviceID != deviceID }
            .sorted { ($0.online ? 0 : 1, $0.name) < ($1.online ? 0 : 1, $1.name) }
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

    func start() {
        guard pollTask == nil else { return }
        pollTask = Task { [weak self] in
            while !Task.isCancelled {
                await self?.refresh()
                try? await Task.sleep(for: .seconds(2))
            }
        }
    }

    func refresh() async {
        do {
            state = try await client.state()
        } catch {
            state = nil
        }
    }

    func togglePaused() {
        guard let state else { return }
        Task {
            try? await client.setPaused(!state.paused)
            await refresh()
        }
    }

    func copyCurrentClip() {
        guard let clip = state?.clip, clip.isText, let content = clip.content else { return }
        NSPasteboard.general.clearContents()
        NSPasteboard.general.setString(content, forType: .string)
    }

    func startEngine() {
        Task {
            do {
                try await EngineServiceManager.shared.activate(forceRestart: true)
                errorMessage = nil
            } catch {
                UserDefaults.standard.set(
                    error.localizedDescription,
                    forKey: EngineServiceManager.lastErrorKey
                )
                errorMessage = error.localizedDescription
                if case EngineServiceError.approvalRequired = error {
                    EngineServiceManager.shared.openLoginItemsSettings()
                }
            }
            try? await Task.sleep(for: .seconds(1))
            await refresh()
        }
    }
}
