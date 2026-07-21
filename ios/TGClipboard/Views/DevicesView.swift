import SwiftUI
import TGClipboardKit

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
            .animation(reduceMotion ? .none : .tgSpring, value: viewModel.errorMessage != nil)
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
                    .contextMenu {
                        // Removing an online device is pointless — it re-registers
                        // on its next heartbeat — so the action only shows for
                        // offline strays (reinstalls, retired hardware).
                        if !device.online {
                            Button(role: .destructive) {
                                Task { await viewModel.removeDevice(device) }
                            } label: {
                                Label("Remove Device", systemImage: "trash")
                            }
                        }
                    }
            }
        }
        .accessibilityLabel("Your devices")
    }

    // MARK: - Empty state

    private var emptyState: some View {
        VStack(spacing: 16) {
            ZStack {
                Circle()
                    .fill(Color.accentColor.opacity(0.12))
                    .frame(width: 96, height: 96)
                Image(systemName: "laptopcomputer.and.iphone")
                    .font(.system(size: 40))
                    .foregroundStyle(Color.accentColor)
            }
            VStack(spacing: 6) {
                Text("Your devices show up here")
                    .font(.title3.weight(.semibold))
                Text("Install tg-clipboard on your Mac and it finds this iPhone automatically — no setup, no pairing codes.")
                    .font(.subheadline)
                    .foregroundStyle(.secondary)
                    .multilineTextAlignment(.center)
            }
            Link("Set up my Mac", destination: URL(string: "https://github.com/thalysguimaraes/tg-clipboard")!)
                .buttonStyle(.borderedProminent)
        }
        .padding(.horizontal, 32)
        .padding(.top, 60)
        .frame(maxWidth: .infinity)
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

/// One device in the grid: a friendly colored avatar (Blip-style), name,
/// and reachability — the sibling of the macOS popover tile (spec §4.2).
/// The avatar hue is derived from the device ID, so "Thalys's MacBook" is
/// the same purple on every screen and every platform.
private struct DeviceTile: View {
    let device: Device

    private var avatarColor: Color {
        Color(hue: device.avatarHue, saturation: 0.55, brightness: 0.85)
    }

    var body: some View {
        VStack(spacing: 10) {
            Image(systemName: device.platformSymbol)
                .font(.system(size: 26, weight: .medium))
                .foregroundStyle(.white)
                .frame(width: 64, height: 64)
                .background(avatarColor.gradient, in: Circle())
                .overlay(alignment: .bottomTrailing) {
                    Circle()
                        .fill(device.online ? Color.green : Color(.systemGray3))
                        .frame(width: 16, height: 16)
                        .overlay(
                            Circle().stroke(Color(.secondarySystemGroupedBackground), lineWidth: 3)
                        )
                }
                .saturation(device.online ? 1 : 0.35)
                .accessibilityHidden(true)

            VStack(spacing: 2) {
                Text(device.name)
                    .font(.subheadline.weight(.semibold))
                    .lineLimit(1)
                    .truncationMode(.tail)

                Text(device.online
                     ? "Online"
                     : device.lastSeen.formatted(.relative(presentation: .named)))
                    .font(.caption)
                    .foregroundStyle(.secondary)
            }
        }
        .frame(maxWidth: .infinity)
        .padding(.vertical, 20)
        .padding(.horizontal, 8)
        .background(
            Color(.secondarySystemGroupedBackground),
            in: RoundedRectangle(cornerRadius: 20, style: .continuous)
        )
        .accessibilityElement(children: .combine)
        .accessibilityLabel("\(device.name), \(device.platformLabel), \(device.online ? "online" : "last seen \(device.lastSeen.formatted(.relative(presentation: .named)))")")
    }
}
