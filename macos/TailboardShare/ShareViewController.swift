import AppKit
import SwiftUI
import UniformTypeIdentifiers

final class ShareViewController: NSViewController {
    private let viewModel = ShareExtensionViewModel()

    override func loadView() {
        view = NSView(frame: NSRect(x: 0, y: 0, width: 380, height: 250))
    }

    override func viewDidLoad() {
        super.viewDidLoad()
        let root = ShareExtensionView(
            viewModel: viewModel,
            cancel: { [weak self] in self?.cancel() },
            complete: { [weak self] in self?.complete() }
        )
        let host = NSHostingController(rootView: root)
        addChild(host)
        host.view.frame = view.bounds
        host.view.autoresizingMask = [.width, .height]
        view.addSubview(host.view)

        Task {
            let files = await Self.materializeSharedFiles(
                from: extensionContext?.inputItems ?? []
            )
            await viewModel.prepare(files: files)
        }
    }

    private func complete() {
        viewModel.cleanup()
        extensionContext?.completeRequest(returningItems: nil)
    }

    private func cancel() {
        viewModel.cleanup()
        extensionContext?.cancelRequest(
            withError: NSError(
                domain: "TailboardShare",
                code: NSUserCancelledError,
                userInfo: nil
            )
        )
    }

    private static func materializeSharedFiles(from inputItems: [Any]) async -> [URL] {
        var files: [URL] = []
        for case let item as NSExtensionItem in inputItems {
            for provider in item.attachments ?? [] {
                guard let source = await fileURL(from: provider) else { continue }
                let destination = FileManager.default.temporaryDirectory
                    .appendingPathComponent("tailboard-share-\(UUID().uuidString)", isDirectory: true)
                    .appendingPathComponent(source.lastPathComponent)
                do {
                    try FileManager.default.createDirectory(
                        at: destination.deletingLastPathComponent(),
                        withIntermediateDirectories: true
                    )
                    let accessed = source.startAccessingSecurityScopedResource()
                    defer { if accessed { source.stopAccessingSecurityScopedResource() } }
                    try FileManager.default.copyItem(at: source, to: destination)
                    files.append(destination)
                } catch {
                    continue
                }
            }
        }
        return files
    }

    private static func fileURL(from provider: NSItemProvider) async -> URL? {
        guard provider.hasItemConformingToTypeIdentifier(UTType.fileURL.identifier) else {
            return nil
        }
        return await withCheckedContinuation { continuation in
            provider.loadItem(
                forTypeIdentifier: UTType.fileURL.identifier,
                options: nil
            ) { item, _ in
                if let url = item as? URL {
                    continuation.resume(returning: url)
                } else if let url = item as? NSURL {
                    continuation.resume(returning: url as URL)
                } else if let data = item as? Data,
                          let string = String(data: data, encoding: .utf8),
                          let url = URL(string: string) {
                    continuation.resume(returning: url)
                } else {
                    continuation.resume(returning: nil)
                }
            }
        }
    }
}
