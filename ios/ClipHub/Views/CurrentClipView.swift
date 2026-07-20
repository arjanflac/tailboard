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
                        ContentUnavailableView(
                            "No Clipboard Content",
                            systemImage: "doc.on.clipboard",
                            description: Text("Copy something on another device and it appears here.")
                        )
                        .padding(.top, 40)
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

    // MARK: - Clip card

    @ViewBuilder
    private func clipCard(_ clip: ClipItem) -> some View {
        VStack(alignment: .leading, spacing: 12) {
            HStack {
                Label(clip.displaySummary, systemImage: clip.kindSymbol)
                    .font(.footnote.weight(.medium))
                    .foregroundStyle(.secondary)

                Spacer()

                Text("from \(clip.source)")
                    .font(.footnote)
                    .foregroundStyle(.secondary)
            }

            clipContent(clip)

            if clip.expiresAt > .now {
                Text("Expires \(clip.expiresAt, style: .relative)")
                    .font(.footnote)
                    .foregroundStyle(.secondary)
            } else {
                Text("Expired \(clip.expiresAt, style: .relative)")
                    .font(.footnote)
                    .foregroundStyle(.secondary)
            }

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
            in: RoundedRectangle(cornerRadius: 12, style: .continuous)
        )
        .overlay {
            if contrast == .increased {
                RoundedRectangle(cornerRadius: 12, style: .continuous)
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
