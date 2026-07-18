import Foundation
import UniformTypeIdentifiers
import ClipHubKit

@Observable
final class ShareViewModel {
    private let store = AppGroupStore()

    var previewText = "Loading..."
    var mimeType = "text/plain"
    var contentToSend: String?
    var dataToSend: Data?
    var suggestedName = "Shared Item"
    var devices: [Device] = []
    var selectedDeviceID = ""
    var isSending = false
    var didSend = false
    var errorMessage: String?

    func extractContent(from items: [NSExtensionItem]) async {
        for item in items {
            guard let providers = item.attachments else { continue }
            for provider in providers {
                // Text
                if provider.hasItemConformingToTypeIdentifier(UTType.plainText.identifier) {
                    if let text = try? await provider.loadItem(forTypeIdentifier: UTType.plainText.identifier) as? String {
                        contentToSend = text
                        mimeType = "text/plain"
                        previewText = String(text.prefix(200))
                        suggestedName = provider.suggestedName ?? "Shared Text.txt"
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
                        return
                    }
                }
                // Image
                if provider.hasItemConformingToTypeIdentifier(UTType.image.identifier) {
                    if let data = try? await provider.loadDataRepresentation(for: .png) {
                        dataToSend = data
                        mimeType = "image/png"
                        previewText = "[Image, \(data.count) bytes]"
                        suggestedName = provider.suggestedName ?? "Shared Image.png"
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
                    previewText = "[\(type.localizedDescription ?? "File"), \(data.count) bytes]"
                    suggestedName = provider.suggestedName ?? "Shared File"
                    return
                }
            }
        }
        previewText = "No shareable content found"
    }

    func loadDevices() async {
        guard let hubURL = store.hubURL else { return }
        let client = ClipHubClient(baseURL: hubURL, sourceName: store.sourceName)
        do {
            devices = try await client.getDevices().filter {
                $0.deviceID != store.deviceID && $0.capabilities.contains("transfers")
            }
        } catch {
            // Clipboard send remains available when the roster is offline.
        }
    }

    func send() async {
        guard let hubURL = store.hubURL else {
            errorMessage = "Open ClipHub to configure"
            return
        }

        isSending = true
        defer { isSending = false }

        let client = ClipHubClient(baseURL: hubURL, sourceName: store.sourceName)

        do {
            if !selectedDeviceID.isEmpty {
                try await sendTransfer(client: client, hubURL: hubURL)
                didSend = true
                return
            }
            if let content = contentToSend {
                _ = try await client.postClip(content: content, mimeType: mimeType)
            } else if let data = dataToSend {
                _ = try await client.postClip(data: data, mimeType: mimeType)
            } else {
                errorMessage = "Nothing to send"
                return
            }
            didSend = true
        } catch {
            errorMessage = error.localizedDescription
        }
    }

    private func sendTransfer(client: ClipHubClient, hubURL: URL) async throws {
        let data: Data
        if let binary = dataToSend {
            data = binary
        } else if let content = contentToSend {
            data = Data(content.utf8)
        } else {
            throw ClipHubError.emptyClipboard
        }

        let created = try await client.createTransfer(
            deviceID: store.deviceID,
            toDevice: selectedDeviceID,
            fileName: URL(fileURLWithPath: suggestedName).lastPathComponent,
            size: Int64(data.count),
            mimeType: mimeType,
            sha256: ClipHash.sha256Hex(data)
        )
        guard let uploadPath = created.uploadURLs.first,
              let uploadURL = URL(string: uploadPath, relativeTo: hubURL)?.absoluteURL,
              let container = FileManager.default.containerURL(
                forSecurityApplicationGroupIdentifier: AppGroupStore.suiteName
              ) else {
            throw ClipHubError.noHubURL
        }
        let directory = container.appendingPathComponent("BackgroundUploads", isDirectory: true)
        try FileManager.default.createDirectory(
            at: directory,
            withIntermediateDirectories: true
        )
        let staged = directory.appendingPathComponent("\(created.transfer.transferID)-0.upload")
        try data.write(to: staged, options: .atomic)

        var request = URLRequest(url: uploadURL)
        request.httpMethod = "PUT"
        request.setValue(store.deviceID, forHTTPHeaderField: "X-Clip-Device-ID")
        request.setValue(mimeType, forHTTPHeaderField: "Content-Type")
        request.setValue(
            "bytes 0-*/\(data.count)",
            forHTTPHeaderField: "Content-Range"
        )
        BackgroundUploadCoordinator.shared.schedule(request: request, fileURL: staged)
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
                    continuation.resume(throwing: error ?? ClipHubError.emptyClipboard)
                }
            }
        }
    }
}
