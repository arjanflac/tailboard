import SwiftUI
import ClipHubKit

/// The LocalSend heart of the app (spec §5.2): a tile grid of your devices,
/// with incoming transfer offers on top. Tiles are status-first today; the
/// send sheet arrives with in-app sending (phase 5).
struct DevicesView: View {
    @Environment(AppViewModel.self) private var viewModel
    @Environment(\.accessibilityReduceMotion) private var reduceMotion

    private var incoming: [Transfer] {
        viewModel.transfers.filter { $0.state == "offered" }
    }

    private let columns = [
        GridItem(.flexible(), spacing: 12),
        GridItem(.flexible(), spacing: 12)
    ]

    var body: some View {
        NavigationStack {
            ScrollView {
                VStack(alignment: .leading, spacing: 16) {
                    if !incoming.isEmpty {
                        incomingSection
                    }

                    if viewModel.devices.isEmpty {
                        if incoming.isEmpty && !viewModel.isLoading {
                            emptyState
                        }
                    } else {
                        deviceGrid
                    }
                }
                .padding()
            }
            .background(Color(.systemGroupedBackground))
            .navigationTitle("Devices")
            .toolbar {
                ToolbarItem(placement: .topBarTrailing) {
                    Button {
                        Task { await viewModel.refresh() }
                    } label: {
                        Image(systemName: "arrow.clockwise")
                    }
                    .accessibilityLabel("Refresh devices")
                }
            }
            .refreshable { await viewModel.refresh() }
            .overlay(alignment: .top) { errorOverlay }
            .animation(reduceMotion ? .none : .clipHub, value: viewModel.errorMessage != nil)
            .task { await viewModel.refresh() }
        }
    }

    // MARK: - Incoming offers

    private var incomingSection: some View {
        VStack(alignment: .leading, spacing: 8) {
            Text("Incoming")
                .font(.headline)

            VStack(spacing: 0) {
                ForEach(incoming) { transfer in
                    TransferRowView(
                        transfer: transfer,
                        fromName: deviceName(transfer.fromDevice),
                        isReceiving: viewModel.receivingTransferIDs.contains(transfer.id),
                        accept: { Task { await viewModel.accept(transfer) } },
                        decline: { Task { await viewModel.decline(transfer) } }
                    )
                    .padding(.horizontal)
                    if transfer.id != incoming.last?.id {
                        Divider()
                    }
                }
            }
            .background(
                Color(.secondarySystemGroupedBackground),
                in: RoundedRectangle(cornerRadius: 12, style: .continuous)
            )
        }
    }

    // MARK: - Device grid

    private var deviceGrid: some View {
        LazyVGrid(columns: columns, spacing: 12) {
            ForEach(viewModel.devices) { device in
                DeviceTile(device: device)
            }
        }
        .accessibilityLabel("Your devices")
    }

    // MARK: - Empty state

    private var emptyState: some View {
        ContentUnavailableView {
            Label("No Devices Yet", systemImage: "laptopcomputer.and.iphone")
        } description: {
            Text("Your devices appear automatically. Install ClipHub on your Mac to get started.")
        } actions: {
            Link("Set up ClipHub on your Mac", destination: URL(string: "https://github.com/thalysguimaraes/cliphub")!)
        }
        .padding(.top, 40)
    }

    // MARK: - Error overlay

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

    private func deviceName(_ id: String) -> String {
        viewModel.devices.first(where: { $0.deviceID == id })?.name ?? "Unknown device"
    }
}

/// One device in the grid: platform glyph, name, and reachability status —
/// the sibling of the macOS popover tile (spec §4.2).
private struct DeviceTile: View {
    let device: Device

    var body: some View {
        VStack(spacing: 8) {
            Image(systemName: device.platformSymbol)
                .font(.system(size: 28))
                .foregroundStyle(Color.accentColor)
                .frame(width: 48, height: 48)
                .background(Color.accentColor.opacity(0.12), in: Circle())
                .accessibilityHidden(true)

            Text(device.name)
                .font(.subheadline.weight(.medium))
                .lineLimit(1)
                .truncationMode(.tail)

            StatusDot(
                color: device.online ? .green : Color(.systemGray),
                text: device.online
                    ? "Online"
                    : "Last seen \(device.lastSeen.formatted(.relative(presentation: .named)))"
            )
        }
        .frame(maxWidth: .infinity)
        .padding(.vertical, 16)
        .padding(.horizontal, 8)
        .background(
            Color(.secondarySystemGroupedBackground),
            in: RoundedRectangle(cornerRadius: 12, style: .continuous)
        )
        .accessibilityElement(children: .combine)
        .accessibilityLabel("\(device.name), \(device.platformLabel), \(device.online ? "online" : "offline")")
    }
}
