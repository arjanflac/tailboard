import SwiftUI
import TGClipboardKit

/// A history row whose primary action is copy — the whole point of a
/// clipboard history. Tap copies with a checkmark flash + success haptic;
/// long-press offers Copy / Share. Used by the Clipboard home "Recent"
/// section and the pushed History screen.
struct ClipRowView: View {
    let item: ClipItem
    /// Returns true when the clip was actually written to the pasteboard;
    /// the confirmation only fires then (no false success on binary clips).
    let copy: (ClipItem) -> Bool

    @State private var confirmed = false
    @State private var revertTask: Task<Void, Never>?
    @Environment(\.accessibilityReduceMotion) private var reduceMotion

    var body: some View {
        Button(action: performCopy) {
            HStack(spacing: 8) {
                VStack(alignment: .leading, spacing: 4) {
                    Text(item.preview)
                        .font(.body)
                        .foregroundStyle(.primary)
                        .lineLimit(2)
                        .multilineTextAlignment(.leading)

                    HStack(spacing: 8) {
                        Text(item.displaySummary)
                            .font(.caption.weight(.medium))
                            .foregroundStyle(.secondary)
                            .padding(.horizontal, 6)
                            .padding(.vertical, 2)
                            .background(Color(.tertiarySystemFill), in: Capsule())

                        Text(item.source)
                            .font(.footnote)
                            .foregroundStyle(.secondary)

                        Text(item.createdAt, style: .relative)
                            .font(.footnote)
                            .foregroundStyle(.secondary)
                    }
                }

                Spacer(minLength: 0)

                if confirmed {
                    Image(systemName: "checkmark.circle.fill")
                        .foregroundStyle(.green)
                        .symbolEffect(.bounce, value: confirmed)
                        .transition(.opacity)
                }
            }
            .padding(.vertical, 6)
            .contentShape(Rectangle())
        }
        .buttonStyle(.plain)
        .animation(reduceMotion ? .none : .tgSpring, value: confirmed)
        .sensoryFeedback(.success, trigger: confirmed) { _, new in new }
        .contextMenu { contextMenu }
        .accessibilityLabel("\(item.displaySummary), from \(item.source)")
        .accessibilityHint("Double-tap to copy")
        .accessibilityAction(named: "Copy", performCopy)
    }

    @ViewBuilder
    private var contextMenu: some View {
        Button(action: performCopy) {
            Label("Copy", systemImage: "doc.on.doc")
        }
        if item.isText, let content = item.content {
            ShareLink(item: content) {
                Label("Share", systemImage: "square.and.arrow.up")
            }
        } else if item.mimeType == "image/png", let data = item.data,
                  let uiImage = UIImage(data: data) {
            ShareLink(
                item: Image(uiImage: uiImage),
                preview: SharePreview("Image from \(item.source)", image: Image(uiImage: uiImage))
            ) {
                Label("Share", systemImage: "square.and.arrow.up")
            }
        }
    }

    private func performCopy() {
        guard copy(item) else { return }
        confirmed = true
        revertTask?.cancel()
        revertTask = Task { @MainActor in
            try? await Task.sleep(for: .seconds(1.5))
            guard !Task.isCancelled else { return }
            confirmed = false
        }
    }
}
