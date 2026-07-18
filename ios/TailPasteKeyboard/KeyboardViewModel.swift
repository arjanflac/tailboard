import Foundation
import UIKit
import ClipHubKit

@Observable
final class KeyboardViewModel {
    private let store = AppGroupStore()
    private weak var proxy: UITextDocumentProxy?

    var currentClip: ClipItem?
    var recentClips: [ClipItem] = []
    var isLoading = false
    var errorMessage: String?

    init(proxy: UITextDocumentProxy) {
        self.proxy = proxy
        // Load cached data immediately (no network).
        self.currentClip = store.cachedCurrentClip
        self.recentClips = store.cachedRecentClips
    }

    func refresh() {
        guard let hubURL = store.hubURL else {
            errorMessage = "Open ClipHub to set up"
            return
        }

        isLoading = true
        Task {
            let client = ClipHubClient(baseURL: hubURL, sourceName: store.sourceName)
            do {
                if let clip = try await client.getCurrentClip() {
                    await MainActor.run {
                        self.currentClip = clip
                        self.store.cachedCurrentClip = clip
                    }
                }
                let history = try await client.getHistory(limit: 10)
                await MainActor.run {
                    self.recentClips = history
                    self.store.cachedRecentClips = history
                    self.isLoading = false
                    self.errorMessage = nil
                }
            } catch {
                await MainActor.run {
                    self.isLoading = false
                    // Keep cached data visible, just note the error.
                    self.errorMessage = "Offline"
                }
            }
        }
    }

    func insertCurrent() {
        guard let clip = currentClip else { return }
        use(clip)
    }

    func insert(_ clip: ClipItem) {
        use(clip)
    }

    func pushClipboard() {
        guard let hubURL = store.hubURL else {
            errorMessage = "Open ClipHub to set up"
            return
        }
        isLoading = true
        Task {
            let client = ClipHubClient(baseURL: hubURL, sourceName: store.sourceName)
            do {
                if let image = UIPasteboard.general.image,
                   let data = image.pngData() {
                    _ = try await client.postClip(data: data, mimeType: "image/png")
                } else if let text = UIPasteboard.general.string, !text.isEmpty {
                    _ = try await client.postClip(content: text)
                } else {
                    throw ClipHubError.emptyClipboard
                }
                await MainActor.run {
                    self.isLoading = false
                    self.errorMessage = "Pushed"
                }
            } catch {
                await MainActor.run {
                    self.isLoading = false
                    self.errorMessage = "Push failed"
                }
            }
        }
    }

    private func use(_ clip: ClipItem) {
        if clip.isText, let content = clip.content {
            proxy?.insertText(content)
            return
        }
        if clip.mimeType == "image/png", let data = clip.data,
           let image = UIImage(data: data) {
            UIPasteboard.general.image = image
            errorMessage = "Image copied"
        }
    }
}
