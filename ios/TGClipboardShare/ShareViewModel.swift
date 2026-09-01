import Foundation
import TGClipboardKit
import UniformTypeIdentifiers

enum SendState: Equatable {
    case extracting
    case ready
    case sending
    case sent
    case failed(String)
}

/// Clipboard-only share extension. Photos and files belong to Taildrop; this
/// target remains useful for sending selected text and links to every device.
@MainActor
@Observable
final class ShareViewModel {
    private let store = AppGroupStore()

    var state: SendState = .extracting
    var textToSend = ""

    var hasContent: Bool {
        !textToSend.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty
    }

    func extractContent(from items: [NSExtensionItem]) async {
        for item in items {
            guard let providers = item.attachments else { continue }
            for provider in providers {
                if provider.hasItemConformingToTypeIdentifier(UTType.plainText.identifier),
                   let text = try? await provider.loadItem(
                    forTypeIdentifier: UTType.plainText.identifier
                   ) as? String {
                    textToSend = text
                    state = .ready
                    await send()
                    return
                }
                if provider.hasItemConformingToTypeIdentifier(UTType.url.identifier),
                   let url = try? await provider.loadItem(
                    forTypeIdentifier: UTType.url.identifier
                   ) as? URL {
                    textToSend = url.absoluteString
                    state = .ready
                    await send()
                    return
                }
            }
        }
        state = .failed("Share text or a link with Tailboard. Use Tailscale for photos and files.")
    }

    func send() async {
        guard hasContent else { return }
        guard let hubURL = store.hubURL else {
            state = .failed("Open Tailboard once to finish setup, then try again.")
            return
        }
        state = .sending
        do {
            let client = TGClipboardClient(baseURL: hubURL, sourceName: store.sourceName)
            _ = try await client.postClip(content: textToSend)
            state = .sent
        } catch {
            state = .failed(UserFacingError.message(error))
        }
    }
}
