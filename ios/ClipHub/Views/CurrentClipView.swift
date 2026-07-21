import SwiftUI
import ClipHubKit

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
                .animation(reduceMotion ? .none : .clipHub, value: viewModel.copiedBannerVisible)
                .animation(reduceMotion ? .none : .clipHub, value: viewModel.connectionState)
            }
            .background(Color(.systemGroupedBackground))
            .navigationTitle("Clipboard")
            .toolbar { liveActivityButton }
            .refreshable { await viewModel.refresh() }
            .overlay(alignment: .top) { errorOverlay }
            .animation(reduceMotion ? .none : .clipHub, value: viewModel.errorMessage != nil)
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

    // MARK: - Clip card

    @ViewBuilder
    private func clipCard(_ clip: ClipItem) -> some View {
        VStack(alignment: .leading, spacing: 12) {
            HStack(spacing: 8) {
                Image(systemName: clip.kindSymbol)
                    .font(.system(size: 15, weight: .medium))
                    .foregroundStyle(Color.accentColor)
                    .frame(width: 32, height: 32)
                    .background(Color.accentColor.opacity(0.12), in: Circle())
                    .accessibilityHidden(true)

                VStack(alignment: .leading, spacing: 1) {
                    Text(clip.displaySummary)
                        .font(.footnote.weight(.semibold))
                    Text("from \(clip.source) · \(clip.createdAt, style: .relative) ago")
                        .font(.caption)
                        .foregroundStyle(.secondary)
                }

                Spacer()
            }

            clipContent(clip)

            // Primary action row: Copy / Send / Share (spec §5.1).
            HStack(spacing: 8) {
                ConfirmingActionButton(
                    title: "Copy",
                    confirmedTitle: "Copied",
                    systemImage: "doc.on.doc",
                    prominent: true
                ) {
                    viewModel.copyToPasteboard(clip)
                }

                ConfirmingActionButton(
                    title: "Send clipboard",
                    confirmedTitle: "Sent",
                    systemImage: "paperplane"
                ) {
                    await viewModel.pushPasteboardToHub()
                }

                if clip.isText, let content = clip.content {
                    ShareLink(item: content) {
                        Label("Share", systemImage: "square.and.arrow.up")
                    }
                    .buttonStyle(.bordered)
                } else if clip.mimeType == "image/png", let data = clip.data,
                   let uiImage = UIImage(data: data) {
                    ShareLink(
                        item: Image(uiImage: uiImage),
                        preview: SharePreview(
                            "Image from \(clip.source)",
                            image: Image(uiImage: uiImage)
                        )
                    ) {
                        Label("Share", systemImage: "square.and.arrow.up")
                    }
                    .buttonStyle(.bordered)
                }
            }
        }
        .padding()
        .background(
            Color(.secondarySystemGroupedBackground),
            in: RoundedRectangle(cornerRadius: 20, style: .continuous)
        )
        .overlay {
            if contrast == .increased {
                RoundedRectangle(cornerRadius: 20, style: .continuous)
                    .stroke(Color(.separator), lineWidth: 1)
            }
        }
    }

    @ViewBuilder
    private func clipContent(_ clip: ClipItem) -> some View {
        if clip.isText, let content = clip.content {
            Text(content)
                .font(.body)
                .textSelection(.enabled)
                .frame(maxWidth: .infinity, alignment: .leading)
        } else if clip.mimeType == "image/png", let data = clip.data,
                  let uiImage = UIImage(data: data) {
            Image(uiImage: uiImage)
                .resizable()
                .aspectRatio(contentMode: .fit)
                .cornerRadius(8)
                .accessibilityLabel("Image clip from \(clip.source)")
        } else {
            Label("Binary clip · \(clip.formattedByteCount)", systemImage: "doc")
                .font(.body)
                .foregroundStyle(.secondary)
        }
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

    // MARK: - Toolbar & overlays

    private var liveActivityButton: some ToolbarContent {
        ToolbarItem(placement: .topBarTrailing) {
            Button {
                Task {
                    if viewModel.liveActivityRunning {
                        await viewModel.stopLiveActivity()
                    } else {
                        await viewModel.startLiveActivity()
                    }
                }
            } label: {
                Label(
                    "Live Activity",
                    systemImage: viewModel.liveActivityRunning
                        ? "stop.circle.fill"
                        : "dot.radiowaves.left.and.right"
                )
            }
            .disabled(viewModel.currentClip == nil && !viewModel.liveActivityRunning)
            .accessibilityLabel(viewModel.liveActivityRunning ? "Stop Live Activity" : "Start Live Activity")
            .accessibilityHint("Pins the current clip to your Lock Screen and Dynamic Island")
        }
    }

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
