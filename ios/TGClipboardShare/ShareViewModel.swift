import Foundation
import UIKit
import ImageIO
import UniformTypeIdentifiers
import TGClipboardKit

/// The share sheet's explicit state machine. Every phase has its own UI;
/// `failed` keeps the send button available so retry is the same control.
enum SendState: Equatable {
    case extracting
    case ready
    case sending
    case sent
    case failed(String)
}

@MainActor
@Observable
final class ShareViewModel {
    private let store = AppGroupStore()

    var state: SendState = .extracting
    var previewText = ""
    var mimeType = "text/plain"
    var contentToSend: String?
    var dataToSend: Data?
    var suggestedName = "Shared Item"
    /// Downscaled, memory-safe preview for image shares (max 480 px).
    var thumbnail: UIImage?
    var devices: [Device] = []
    var selectedDeviceID = ""

    var hasContent: Bool { contentToSend != nil || dataToSend != nil }

    /// Humanized type label ("Text", "Link", "Image · 2.3 MB") — shared logic
    /// with ClipItem so every surface speaks the same language.
    var contentSummary: String {
        let bytes = dataToSend?.count ?? contentToSend?.utf8.count ?? 0
        return ClipItem.displaySummary(mimeType: mimeType, byteCount: bytes, textContent: contentToSend)
    }

    /// Name of the currently selected destination for the send button and
    /// success message. The no-device selection sends to every device via
    /// the shared clipboard.
    var destinationName: String {
        guard !selectedDeviceID.isEmpty else { return "All my devices (clipboard)" }
        return devices.first(where: { $0.deviceID == selectedDeviceID })?.name ?? "device"
    }

    // MARK: - Extraction

    func extractContent(from items: [NSExtensionItem]) async {
        for item in items {
            guard let providers = item.attachments else { continue }
            for provider in providers {
                // Text
                if provider.hasItemConformingToTypeIdentifier(UTType.plainText.identifier) {
                    if let text = try? await provider.loadItem(forTypeIdentifier: UTType.plainText.identifier) as? String {
                        contentToSend = text
                        mimeType = "text/plain"
                        previewText = String(text.prefix(500))
                        suggestedName = provider.suggestedName ?? "Shared Text.txt"
                        state = .ready
                        return
                    }
                }
                // URL
                if provider.hasItemConformingToTypeIdentifier(UTType.url.identifier) {
                    if let url = try? await provider.loadItem(forTypeIdentifier: UTType.url.identifier) as? URL {
                        contentToSend = url.absoluteString
                        mimeType = "text/plain"
                        previewText = url.absoluteString
                        suggestedName = provider.suggestedName ?? "Shared Link.txt"
                        state = .ready
                        return
                    }
                }
                // Image
                if provider.hasItemConformingToTypeIdentifier(UTType.image.identifier) {
                    if let data = try? await provider.loadDataRepresentation(for: .png) {
                        dataToSend = data
                        mimeType = "image/png"
                        previewText = ""
                        thumbnail = Self.makeThumbnail(from: data)
                        suggestedName = provider.suggestedName ?? "Shared Image.png"
                        state = .ready
                        return
                    }
                }
                // Generic files: stream the provider representation into memory and
                // use its declared content type. Transfer uploads handle larger files.
                if provider.hasItemConformingToTypeIdentifier(UTType.data.identifier),
                   let registeredType = provider.registeredTypeIdentifiers.first,
                   let type = UTType(registeredType),
                   let data = try? await provider.loadDataRepresentation(for: type) {
                    dataToSend = data
                    mimeType = type.preferredMIMEType ?? "application/octet-stream"
                    previewText = ""
                    suggestedName = provider.suggestedName ?? "Shared File"
                    state = .ready
                    return
                }
            }
        }
        previewText = ""
        state = .ready // hasContent stays false; the view explains it
    }

    // MARK: - Devices

    func loadDevices() async {
        guard let hubURL = store.hubURL else { return }
        let client = TGClipboardClient(baseURL: hubURL, sourceName: store.sourceName)
        do {
            devices = try await client.getDevices()
                .filter { $0.deviceID != store.deviceID && $0.capabilities.contains("transfers") }
                .sorted { $0.online && !$1.online }
        } catch {
            // Clipboard send remains available when the roster is offline.
        }
    }

    // MARK: - Send

    func send() async {
        guard hasContent else { return }
        guard let hubURL = store.hubURL else {
            state = .failed("Open tg-clipboard once to finish setup, then try again.")
            return
        }

        state = .sending
        let client = TGClipboardClient(baseURL: hubURL, sourceName: store.sourceName)

        do {
            if !selectedDeviceID.isEmpty {
                try await sendTransfer(client: client)
            } else if let content = contentToSend {
                _ = try await client.postClip(content: content, mimeType: mimeType)
            } else if let data = dataToSend {
                _ = try await client.postClip(data: data, mimeType: mimeType)
            }
            state = .sent
        } catch {
            state = .failed(UserFacingError.message(error))
        }
    }

    private func sendTransfer(client: TGClipboardClient) async throws {
        let data: Data
        if let binary = dataToSend {
            data = binary
        } else if let content = contentToSend {
            data = Data(content.utf8)
        } else {
            throw TGClipboardError.emptyClipboard
        }

        _ = try await client.scheduleTransferUpload(
            deviceID: store.deviceID,
            toDevice: selectedDeviceID,
            data: data,
            fileName: URL(fileURLWithPath: suggestedName).lastPathComponent,
            mimeType: mimeType
        )
    }

    // MARK: - Helpers

    /// ImageIO thumbnail: decodes at most maxPixel points per side instead of
    /// the full image, keeping the extension's memory budget intact.
    private static func makeThumbnail(from data: Data, maxPixel: Int = 480) -> UIImage? {
        let sourceOptions = [kCGImageSourceShouldCache: false] as CFDictionary
        guard let source = CGImageSourceCreateWithData(data as CFData, sourceOptions) else { return nil }
        let thumbOptions = [
            kCGImageSourceCreateThumbnailFromImageAlways: true,
            kCGImageSourceCreateThumbnailWithTransform: true,
            kCGImageSourceThumbnailMaxPixelSize: maxPixel
        ] as CFDictionary
        guard let cgImage = CGImageSourceCreateThumbnailAtIndex(source, 0, thumbOptions) else { return nil }
        return UIImage(cgImage: cgImage)
    }
}

// Helper for loading PNG data from NSItemProvider.
extension NSItemProvider {
    func loadDataRepresentation(for type: UTType) async throws -> Data {
        try await withCheckedThrowingContinuation { continuation in
            _ = loadDataRepresentation(forTypeIdentifier: type.identifier) { data, error in
                if let data {
                    continuation.resume(returning: data)
                } else {
                    continuation.resume(throwing: error ?? TGClipboardError.emptyClipboard)
                }
            }
        }
    }
}
