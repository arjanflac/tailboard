import AppIntents
import TGClipboardKit

struct SendToHubIntent: AppIntent {
    static var title: LocalizedStringResource = "Send Text to Devices"
    static var description = IntentDescription("Sends text to the clipboard on all your devices.")

    @Parameter(title: "Text")
    var text: String

    func perform() async throws -> some IntentResult {
        let store = AppGroupStore()
        guard let hubURL = store.hubURL else { throw TGClipboardError.noHubURL }
        let client = TGClipboardClient(baseURL: hubURL, sourceName: store.sourceName)
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
        guard let hubURL = store.hubURL else { throw TGClipboardError.noHubURL }
        let client = TGClipboardClient(baseURL: hubURL, sourceName: store.sourceName)
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

struct TGClipboardShortcuts: AppShortcutsProvider {
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
            intent: ReceiveClipboardIntent(),
            phrases: ["Receive clipboard with \(.applicationName)"],
            shortTitle: "Receive Clipboard",
            systemImageName: "arrow.down.doc"
        )
        AppShortcut(
            intent: SendFileToDeviceIntent(),
            phrases: ["Send a file with \(.applicationName)"],
            shortTitle: "Send File",
            systemImageName: "paperplane"
        )
    }
}
