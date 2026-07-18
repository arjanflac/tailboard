import SwiftUI
import ClipHubKit

struct DevicesView: View {
    @Environment(AppViewModel.self) private var viewModel

    private var incoming: [Transfer] {
        viewModel.transfers.filter { $0.state == "offered" }
    }

    var body: some View {
        NavigationStack {
            List {
                if !incoming.isEmpty {
                    Section("Incoming") {
                        ForEach(incoming) { transfer in
                            VStack(alignment: .leading, spacing: 8) {
                                Text(transfer.files.map(\.name).joined(separator: ", "))
                                    .lineLimit(2)
                                Text("\(transfer.files.count) file(s) from \(deviceName(transfer.fromDevice))")
                                    .font(.caption)
                                    .foregroundStyle(.secondary)
                                HStack {
                                    Button("Accept") {
                                        Task { await viewModel.accept(transfer) }
                                    }
                                    .buttonStyle(.borderedProminent)
                                    .disabled(viewModel.receivingTransferIDs.contains(transfer.id))
                                    Button("Decline", role: .destructive) {
                                        Task { await viewModel.decline(transfer) }
                                    }
                                    .buttonStyle(.bordered)
                                }
                            }
                            .padding(.vertical, 4)
                        }
                    }
                }

                Section("Devices") {
                    ForEach(viewModel.devices) { device in
                        HStack(spacing: 12) {
                            Image(systemName: platformIcon(device.platform))
                                .frame(width: 24)
                            VStack(alignment: .leading) {
                                Text(device.name)
                                Text(device.platform)
                                    .font(.caption)
                                    .foregroundStyle(.secondary)
                            }
                            Spacer()
                            Circle()
                                .fill(device.online ? .green : .gray)
                                .frame(width: 9, height: 9)
                                .accessibilityLabel(device.online ? "Online" : "Offline")
                        }
                    }
                }
            }
            .overlay {
                if viewModel.devices.isEmpty && incoming.isEmpty && !viewModel.isLoading {
                    ContentUnavailableView(
                        "No Devices Yet",
                        systemImage: "laptopcomputer.and.iphone",
                        description: Text("Start clipd on another device, then refresh.")
                    )
                }
            }
            .navigationTitle("Devices")
            .toolbar {
                Button {
                    Task { await viewModel.refresh() }
                } label: {
                    Image(systemName: "arrow.clockwise")
                }
            }
            .refreshable { await viewModel.refresh() }
            .task { await viewModel.refresh() }
        }
    }

    private func deviceName(_ id: String) -> String {
        viewModel.devices.first(where: { $0.deviceID == id })?.name ?? id
    }

    private func platformIcon(_ platform: String) -> String {
        switch platform {
        case "ios": "iphone"
        case "darwin": "laptopcomputer"
        case "windows": "desktopcomputer"
        case "linux": "terminal"
        default: "display"
        }
    }
}
