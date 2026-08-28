import Foundation
import UIKit
import TGClipboardKit

/// Transient feedback shown in the keyboard's status row. Distinct from the
/// persistent setup hint shown when the hub URL is missing.
enum PasteStatus: Equatable {
    case idle
    case working(String)
    case success(String)
    case failure(String)
}

@MainActor
@Observable
final class KeyboardViewModel {
    private let store = AppGroupStore()
    private weak var proxy: UITextDocumentProxy?

    var currentClip: ClipItem?
    var recentClips: [ClipItem] = []
    var status: PasteStatus = .idle
    /// Persistent hint shown when the keyboard isn't configured yet.
    var setupHint: String?

    /// Only text clips appear as insertable chips; images get their own
    /// dedicated action so the same-looking chip never does two things.
    var textClips: [ClipItem] { recentClips.filter(\.isText) }
    var imageClip: ClipItem? { recentClips.first(where: { $0.mimeType.hasPrefix("image/") }) }

    private var loadingGraceTask: Task<Void, Never>?
    private var statusClearTask: Task<Void, Never>?

    init(proxy: UITextDocumentProxy) {
        self.proxy = proxy
        // Load cached data immediately (no network).
        self.currentClip = store.cachedCurrentClip
        self.recentClips = store.cachedRecentClips
        self.setupHint = store.hubURL == nil ? "Open Tailboard to finish setup" : nil
    }

    // MARK: - Refresh

    func refresh() {
        guard let hubURL = store.hubURL else {
            setupHint = "Open Tailboard to finish setup"
            return
        }
        setupHint = nil

        // Only surface a spinner if the fetch outlives a short grace period;
        // cached content is already visible, so fast fetches stay silent.
        loadingGraceTask?.cancel()
        loadingGraceTask = Task { [weak self] in
            try? await Task.sleep(for: .milliseconds(300))
            guard !Task.isCancelled else { return }
            self?.status = .working("Updating…")
        }

        Task { [weak self] in
            guard let self else { return }
            let client = TGClipboardClient(baseURL: hubURL, sourceName: store.sourceName)
            do {
                if let clip = try await client.getCurrentClip() {
                    self.currentClip = clip
                    self.store.cachedCurrentClip = clip
                }
                let history = try await client.getHistory(limit: 10)
                self.recentClips = history
                self.store.cachedRecentClips = history
                self.endLoading()
                self.status = .idle
            } catch {
                self.endLoading()
                // Keep cached data visible, just note the error.
                self.flash(.failure("Offline — showing cached clips"))
            }
        }
    }

    private func endLoading() {
        loadingGraceTask?.cancel()
        loadingGraceTask = nil
        if case .working = status { status = .idle }
    }

    // MARK: - Actions

    func insertCurrent() {
        guard let clip = currentClip else { return }
        use(clip)
    }

    func insert(_ clip: ClipItem) {
        use(clip)
    }

    /// Copies an image clip to the general pasteboard. The keyboard cannot
    /// insert images directly, so this is an explicit, separate action.
    func copyImageClip(_ clip: ClipItem) {
        guard let data = clip.data, let image = UIImage(data: data) else {
            flash(.failure("Couldn't read image"))
            return
        }
        UIPasteboard.general.image = image
        UINotificationFeedbackGenerator().notificationOccurred(.success)
        flash(.success("Image copied — paste it into the app"))
    }

    func pushClipboard() {
        guard let hubURL = store.hubURL else {
            setupHint = "Open Tailboard to finish setup"
            return
        }
        status = .working("Sending…")
        Task { [weak self] in
            guard let self else { return }
            let client = TGClipboardClient(baseURL: hubURL, sourceName: store.sourceName)
            do {
                if let image = UIPasteboard.general.image,
                   let data = image.pngData() {
                    _ = try await client.postClip(data: data, mimeType: "image/png")
                } else if let text = UIPasteboard.general.string, !text.isEmpty {
                    _ = try await client.postClip(content: text)
                } else {
                    throw TGClipboardError.emptyClipboard
                }
                UINotificationFeedbackGenerator().notificationOccurred(.success)
                self.flash(.success("Sent to your other devices"))
            } catch {
                UINotificationFeedbackGenerator().notificationOccurred(.error)
                self.flash(.failure("Send failed — check connection"))
            }
        }
    }

    // MARK: - Private

    private func use(_ clip: ClipItem) {
        if clip.isText, let content = clip.content {
            UIImpactFeedbackGenerator(style: .light).impactOccurred()
            proxy?.insertText(content)
            return
        }
        if clip.mimeType.hasPrefix("image/") {
            copyImageClip(clip)
        }
    }

    /// Shows a transient status and auto-clears successes after a short dwell.
    private func flash(_ newStatus: PasteStatus) {
        statusClearTask?.cancel()
        status = newStatus
        if case .success = newStatus {
            statusClearTask = Task { [weak self] in
                try? await Task.sleep(for: .milliseconds(2500))
                guard !Task.isCancelled else { return }
                if case .success = self?.status { self?.status = .idle }
            }
        }
    }
}
