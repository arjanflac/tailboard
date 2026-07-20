import AppIntents
import UIKit
import ClipHubKit

struct GetCurrentClipIntent: AppIntent {
    static var title: LocalizedStringResource = "Get Current Clip"
    static var description = IntentDescription("Gets the current text clip from your synced clipboard.")

    func perform() async throws -> some IntentResult & ReturnsValue<String> {
        let store = AppGroupStore()
        guard let hubURL = store.hubURL else { throw ClipHubError.noHubURL }
        let client = ClipHubClient(baseURL: hubURL, sourceName: store.sourceName)
        let value = try await client.getCurrentClip()?.content ?? ""
        return .result(value: value)
    }
}

struct PushClipboardIntent: AppIntent {
    static var title: LocalizedStringResource = "Send Clipboard"
    static var description = IntentDescription("Sends this device's clipboard to your other devices.")

    @MainActor
    func perform() async throws -> some IntentResult {
        let store = AppGroupStore()
        guard let hubURL = store.hubURL else { throw ClipHubError.noHubURL }
        let client = ClipHubClient(baseURL: hubURL, sourceName: store.sourceName)
        if let image = UIPasteboard.general.image, let data = image.pngData() {
            _ = try await client.postClip(data: data, mimeType: "image/png")
        } else if let text = UIPasteboard.general.string, !text.isEmpty {
            _ = try await client.postClip(content: text)
        } else {
            throw ClipHubError.emptyClipboard
        }
        return .result()
    }
}

struct SendToHubIntent: AppIntent {
    static var title: LocalizedStringResource = "Send Text to Devices"
    static var description = IntentDescription("Sends text to the clipboard on all your devices.")

    @Parameter(title: "Text")
    var text: String

    func perform() async throws -> some IntentResult {
        let store = AppGroupStore()
        guard let hubURL = store.hubURL else { throw ClipHubError.noHubURL }
        let client = ClipHubClient(baseURL: hubURL, sourceName: store.sourceName)
        _ = try await client.postClip(content: text)
        return .result()
    }
}

struct SendFileToDeviceIntent: AppIntent {
    static var title: LocalizedStringResource = "Send File to Device"
    static var description = IntentDescription("Sends a file to one of your devices.")

    @Parameter(title: "File")
    var file: IntentFile

    @Parameter(title: "Device ID")
    var deviceID: String

    func perform() async throws -> some IntentResult {
        let store = AppGroupStore()
        guard let hubURL = store.hubURL else { throw ClipHubError.noHubURL }
        let client = ClipHubClient(baseURL: hubURL, sourceName: store.sourceName)
        _ = try await client.scheduleTransferUpload(
            deviceID: store.deviceID,
            toDevice: deviceID,
            data: file.data,
            fileName: file.filename,
            mimeType: "application/octet-stream"
        )
        return .result()
    }
}

struct ClipHubShortcuts: AppShortcutsProvider {
    static var appShortcuts: [AppShortcut] {
        AppShortcut(
            intent: PushClipboardIntent(),
            phrases: ["Send clipboard with \(.applicationName)"],
            shortTitle: "Send Clipboard",
            systemImageName: "paperplane"
        )
        AppShortcut(
            intent: GetCurrentClipIntent(),
            phrases: ["Get current clip from \(.applicationName)"],
            shortTitle: "Get Current Clip",
            systemImageName: "doc.on.clipboard"
        )
        AppShortcut(
            intent: SendFileToDeviceIntent(),
            phrases: ["Send a file with \(.applicationName)"],
            shortTitle: "Send File",
            systemImageName: "paperplane"
        )
    }
}
