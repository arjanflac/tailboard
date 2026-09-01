import SwiftUI
import TGClipboardKit

struct CurrentClipView: View {
    @Environment(AppViewModel.self) private var viewModel
    @Environment(\.accessibilityReduceMotion) private var reduceMotion
    @Environment(\.colorSchemeContrast) private var contrast

    var body: some View {
        NavigationStack {
            ScrollView {
                VStack(spacing: 16) {
                    if viewModel.connectionState != .connected {
                        ConnectionBanner(state: viewModel.connectionState)
                            .transition(.opacity.combined(with: .move(edge: .top)))
                    }

                    if viewModel.copiedBannerVisible {
                        CopiedPill()
                            .transition(.opacity.combined(with: .scale(scale: 0.9)))
                    }

                    if let clip = viewModel.currentClip {
                        clipCard(clip)
                    } else {
                        emptyState
                    }

                    if !viewModel.history.isEmpty {
                        recentSection
                    }
                }
                .padding()
                .animation(reduceMotion ? .none : .tgSpring, value: viewModel.copiedBannerVisible)
                .animation(reduceMotion ? .none : .tgSpring, value: viewModel.connectionState)
            }
            .background(Color(.systemGroupedBackground))
            .navigationTitle("Clipboard")
            .refreshable { await viewModel.refresh() }
            .overlay(alignment: .top) { errorOverlay }
            .animation(reduceMotion ? .none : .tgSpring, value: viewModel.errorMessage != nil)
        }
    }

    // MARK: - Empty state

    private var emptyState: some View {
        VStack(spacing: 16) {
            ZStack {
                Circle()
                    .fill(Color.accentColor.opacity(0.12))
                    .frame(width: 96, height: 96)
                Image(systemName: "doc.on.clipboard")
                    .font(.system(size: 40))
                    .foregroundStyle(Color.accentColor)
            }
            VStack(spacing: 6) {
                Text("Nothing here yet")
                    .font(.title3.weight(.semibold))
                Text("Copy something on any of your devices — it lands here in a blink.")
                    .font(.subheadline)
                    .foregroundStyle(.secondary)
                    .multilineTextAlignment(.center)
            }
        }
        .padding(.horizontal, 32)
        .padding(.top, 60)
        .frame(maxWidth: .infinity)
    }

    // MARK: - Hero clip card

    /// The clip is the hero: big content on a gradient card, sender byline
    /// with a colored avatar dot, and one prominent action — Copy. Send and
    /// Share ride along as compact circles.
    @ViewBuilder
    private func clipCard(_ clip: ClipItem) -> some View {
        VStack(alignment: .leading, spacing: 14) {
            heroContent(clip)

            HStack(spacing: 6) {
                Image(systemName: clip.kindSymbol)
                    .font(.caption2.weight(.semibold))
                Text(clip.displaySummary)
                    .font(.caption.weight(.semibold))
                Text("·")
                    .foregroundStyle(.secondary)
                Text("\(clip.source), \(clip.createdAt, style: .relative) ago")
                    .font(.caption)
                    .foregroundStyle(.secondary)
                    .lineLimit(1)
                Spacer(minLength: 0)
            }

            HStack(spacing: 10) {
                ConfirmingActionButton(
                    title: "Copy",
                    confirmedTitle: "Copied",
                    systemImage: "doc.on.doc.fill",
                    prominent: true
                ) {
                    viewModel.copyToPasteboard(clip)
                }

                circleAction("paperplane.fill", label: "Send my clipboard", confirmedIcon: "checkmark") {
                    await viewModel.pushPasteboardToHub()
                }

                shareCircle(clip)
            }
        }
        .padding(18)
        .background {
            RoundedRectangle(cornerRadius: 24, style: .continuous)
                .fill(
                    LinearGradient(
                        colors: [
                            Color.accentColor.opacity(0.14),
                            Color(.secondarySystemGroupedBackground),
                        ],
                        startPoint: .topLeading,
                        endPoint: .bottom
                    )
                )
                .background(
                    Color(.secondarySystemGroupedBackground),
                    in: RoundedRectangle(cornerRadius: 24, style: .continuous)
                )
        }
        .overlay {
            if contrast == .increased {
                RoundedRectangle(cornerRadius: 24, style: .continuous)
                    .stroke(Color(.separator), lineWidth: 1)
            }
        }
    }

    @ViewBuilder
    private func heroContent(_ clip: ClipItem) -> some View {
        if clip.isText, let content = clip.content {
            Text(content)
                .font(content.count <= 80 ? .title3.weight(.medium) : .body)
                .lineLimit(8)
                .textSelection(.enabled)
                .frame(maxWidth: .infinity, alignment: .leading)
        } else if clip.mimeType == "image/png", let data = clip.data,
                  let uiImage = UIImage(data: data) {
            Image(uiImage: uiImage)
                .resizable()
                .aspectRatio(contentMode: .fit)
                .frame(maxHeight: 260)
                .clipShape(RoundedRectangle(cornerRadius: 14, style: .continuous))
                .accessibilityLabel("Image clip from \(clip.source)")
        } else {
            HStack(spacing: 10) {
                Image(systemName: "doc.fill")
                    .font(.title2)
                    .foregroundStyle(Color.accentColor)
                Text(clip.displaySummary)
                    .font(.title3.weight(.medium))
            }
            .frame(maxWidth: .infinity, alignment: .leading)
        }
    }

    /// Compact circular secondary action with a transient ✓ confirmation.
    @ViewBuilder
    private func circleAction(
        _ systemImage: String,
        label: String,
        confirmedIcon: String,
        action: @escaping () async -> Void
    ) -> some View {
        CircleActionButton(systemImage: systemImage, confirmedIcon: confirmedIcon, action: action)
            .accessibilityLabel(label)
    }

    @ViewBuilder
    private func shareCircle(_ clip: ClipItem) -> some View {
        Group {
            if clip.isText, let content = clip.content {
                ShareLink(item: content) {
                    Image(systemName: "square.and.arrow.up")
                        .font(.body.weight(.semibold))
                        .frame(width: 44, height: 44)
                }
            } else if clip.mimeType == "image/png", let data = clip.data,
                      let uiImage = UIImage(data: data) {
                ShareLink(
                    item: Image(uiImage: uiImage),
                    preview: SharePreview("Image from \(clip.source)", image: Image(uiImage: uiImage))
                ) {
                    Image(systemName: "square.and.arrow.up")
                        .font(.body.weight(.semibold))
                        .frame(width: 44, height: 44)
                }
            }
        }
        .buttonStyle(.bordered)
        .clipShape(Circle())
        .accessibilityLabel("Share")
    }

    // MARK: - Recent history

    private var recentSection: some View {
        VStack(alignment: .leading, spacing: 8) {
            HStack {
                Text("Recent")
                    .font(.headline)
                Spacer()
                NavigationLink("Show All") { HistoryView() }
                    .font(.subheadline)
            }

            let recent = Array(viewModel.history.prefix(5))
            VStack(spacing: 0) {
                ForEach(recent) { item in
                    ClipRowView(item: item, copy: viewModel.copyToPasteboard)
                    if item.id != recent.last?.id {
                        Divider()
                    }
                }
            }
            .padding(.horizontal)
            .background(
                Color(.secondarySystemGroupedBackground),
                in: RoundedRectangle(cornerRadius: 12, style: .continuous)
            )
            .overlay {
                if contrast == .increased {
                    RoundedRectangle(cornerRadius: 12, style: .continuous)
                        .stroke(Color(.separator), lineWidth: 1)
                }
            }
        }
    }

    // MARK: - Overlays

    @ViewBuilder
    private var errorOverlay: some View {
        if let message = viewModel.errorMessage {
            ErrorBanner(message: message) { viewModel.errorMessage = nil }
                .padding()
                .transition(.move(edge: .top).combined(with: .opacity))
                .task(id: message) {
                    try? await Task.sleep(for: .seconds(6))
                    guard !Task.isCancelled else { return }
                    viewModel.errorMessage = nil
                }
        }
    }
}
