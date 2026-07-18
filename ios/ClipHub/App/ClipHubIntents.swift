import AppIntents
import UIKit
import ClipHubKit

struct GetCurrentClipIntent: AppIntent {
    static var title: LocalizedStringResource = "Get Current Clip"
    static var description = IntentDescription("Gets the current text clip from your ClipHub.")

    func perform() async throws -> some IntentResult & ReturnsValue<String> {
        let store = AppGroupStore()
        guard let hubURL = store.hubURL else { throw ClipHubError.noHubURL }
        let client = ClipHubClient(baseURL: hubURL, sourceName: store.sourceName)
        let value = try await client.getCurrentClip()?.content ?? ""
        return .result(value: value)
    }
}

struct PushClipboardIntent: AppIntent {
    static var title: LocalizedStringResource = "Push Clipboard"
    static var description = IntentDescription("Sends the iPhone clipboard to your ClipHub.")

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
    static var title: LocalizedStringResource = "Send Text to ClipHub"
    static var description = IntentDescription("Sends text directly to your ClipHub.")

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

struct ClipHubShortcuts: AppShortcutsProvider {
    static var appShortcuts: [AppShortcut] {
        AppShortcut(
            intent: PushClipboardIntent(),
            phrases: ["Push clipboard with \(.applicationName)"],
            shortTitle: "Push Clipboard",
            systemImageName: "arrow.up.doc"
        )
        AppShortcut(
            intent: GetCurrentClipIntent(),
            phrases: ["Get current clip from \(.applicationName)"],
            shortTitle: "Get Current Clip",
            systemImageName: "doc.on.clipboard"
        )
    }
}
