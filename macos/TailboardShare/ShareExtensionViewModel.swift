import Foundation
import Observation
import TGClipboardKit

enum ShareExtensionState: Equatable {
    case loading
    case ready
    case sending(String)
    case sent(String)
    case failed(String)
}

@MainActor
@Observable
final class ShareExtensionViewModel {
    private let client = ControlClient()
    private(set) var files: [URL] = []
    private(set) var devices: [Device] = []
    var state: ShareExtensionState = .loading

    func prepare(files: [URL]) async {
        self.files = files
        guard !files.isEmpty else {
            state = .failed("No readable files were shared.")
            return
        }
        do {
            let controlState = try await client.state()
            devices = (controlState.devices ?? [])
                .filter { $0.deviceID != controlState.deviceID && $0.capabilities.contains("transfers") }
                .sorted {
                    let lhs = ($0.platform == "android" ? 0 : 1, $0.online ? 0 : 1, $0.name)
                    let rhs = ($1.platform == "android" ? 0 : 1, $1.online ? 0 : 1, $1.name)
                    return lhs < rhs
                }
            state = devices.isEmpty ? .failed("No Tailboard devices are registered.") : .ready
        } catch {
            state = .failed("Tailboard Engine isn't available on this Mac.")
        }
    }

    func send(to device: Device) async {
        guard !files.isEmpty else { return }
        state = .sending(device.name)
        do {
            try await client.send(fileURLs: files, to: device.deviceID)
            state = .sent(friendlyName(device))
        } catch {
            state = .failed((error as? ControlError)?.errorDescription ?? "Send failed.")
        }
    }

    func friendlyName(_ device: Device) -> String {
        switch device.platform {
        case "ios": return "iPhone"
        case "android": return device.name.lowercased().contains("pixel") ? "Pixel" : device.name
        default: return device.name
        }
    }

    func cleanup() {
        for file in files {
            try? FileManager.default.removeItem(at: file.deletingLastPathComponent())
        }
        files = []
    }
}
