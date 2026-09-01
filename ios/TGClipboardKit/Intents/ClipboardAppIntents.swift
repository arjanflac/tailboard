#if os(iOS)
import AppIntents
import UIKit

public struct GetCurrentClipIntent: AppIntent {
    public static var title: LocalizedStringResource = "Get Current Clip"
    public static var description = IntentDescription("Gets the current text clip from your synced clipboard.")
    public static var openAppWhenRun: Bool { false }

    public init() {}

    public func perform() async throws -> some IntentResult & ReturnsValue<String> {
        let store = AppGroupStore()
        guard let hubURL = store.hubURL else { throw TGClipboardError.noHubURL }
        let client = TGClipboardClient(baseURL: hubURL, sourceName: store.sourceName)
        let value = try await client.getCurrentClip()?.content ?? ""
        return .result(value: value)
    }
}

public struct ReceiveClipboardIntent: AppIntent {
    public static var title: LocalizedStringResource = "Receive Clipboard"
    public static var description = IntentDescription("Copies the latest synced clip onto this iPhone.")
    // iOS doesn't guarantee pasteboard access to a background intent process.
    // Foreground the app so this action is dependable and visible.
    public static var openAppWhenRun: Bool { true }

    public init() {}

    @MainActor
    public func perform() async throws -> some IntentResult & ProvidesDialog {
        let store = AppGroupStore()
        guard let hubURL = store.hubURL else { throw TGClipboardError.noHubURL }
        let client = TGClipboardClient(baseURL: hubURL, sourceName: store.sourceName)
        guard let clip = try await client.getCurrentClip() else {
            throw TGClipboardError.emptyClipboard
        }

        if clip.isText, let content = clip.content {
            UIPasteboard.general.string = content
        } else if clip.mimeType == "image/png", let data = clip.data {
            UIPasteboard.general.setData(data, forPasteboardType: "public.png")
        } else {
            throw TGClipboardError.unsupportedClipboardType(clip.mimeType)
        }
        return .result(dialog: "Clipboard copied")
    }
}

public struct PushClipboardIntent: AppIntent {
    public static var title: LocalizedStringResource = "Send Clipboard"
    public static var description = IntentDescription("Sends this iPhone's clipboard through Tailboard.")
    // Reading the user's pasteboard is a foreground action on iOS.
    public static var openAppWhenRun: Bool { true }

    public init() {}

    @MainActor
    public func perform() async throws -> some IntentResult & ProvidesDialog {
        let store = AppGroupStore()
        guard let hubURL = store.hubURL else { throw TGClipboardError.noHubURL }
        let client = TGClipboardClient(baseURL: hubURL, sourceName: store.sourceName)
        if let image = UIPasteboard.general.image, let data = image.pngData() {
            _ = try await client.postClip(data: data, mimeType: "image/png")
        } else if let text = UIPasteboard.general.string, !text.isEmpty {
            _ = try await client.postClip(content: text)
        } else {
            throw TGClipboardError.emptyClipboard
        }
        return .result(dialog: "Clipboard sent")
    }
}
#endif
