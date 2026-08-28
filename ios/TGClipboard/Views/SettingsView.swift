import SwiftUI
import UIKit
import TGClipboardKit

struct SettingsView: View {
    @Environment(AppViewModel.self) private var viewModel
    @Environment(\.openURL) private var openURL
    private let store = AppGroupStore()
    @State private var deviceName = AppGroupStore().sourceName
    @State private var controlDiagnostic = AppGroupStore().lastControlDiagnostic
    @State private var defaultTransferDeviceName = AppGroupStore().defaultTransferDeviceName
    @FocusState private var nameFocused: Bool

    var body: some View {
        NavigationStack {
            Form {
                Section("Connection") {
                    LabeledContent("Status") {
                        StatusDot(color: statusColor, text: viewModel.connectionState.label)
                    }

                    LabeledContent("This Device") {
                        TextField("Device name", text: $deviceName)
                            .multilineTextAlignment(.trailing)
                            .focused($nameFocused)
                            .submitLabel(.done)
                            .onSubmit { commitRename() }
                            .onChange(of: nameFocused) { _, focused in
                                if !focused { commitRename() }
                            }
                    }

                    Button("View Devices") {
                        viewModel.selectedTab = .devices
                    }
                }

                Section("Keyboard Extension") {
                    Text("Enable Tailboard Paste in Settings → General → Keyboard → Keyboards → Add New Keyboard → Tailboard Paste.")
                        .font(.footnote)
                        .foregroundStyle(.secondary)

                    Button("Open Settings") {
                        guard let url = URL(string: UIApplication.openSettingsURLString) else { return }
                        openURL(url)
                    }
                }

                Section("File Transfers") {
                    Picker("Default Destination", selection: $defaultTransferDeviceName) {
                        Label("Ask Every Time", systemImage: "questionmark.circle")
                            .tag("")
                        ForEach(transferDevices) { device in
                            Label(device.name, systemImage: device.platformSymbol)
                                .tag(device.name.trimmingCharacters(in: .whitespacesAndNewlines))
                        }
                        if !defaultTransferDeviceName.isEmpty,
                           !transferDevices.contains(where: {
                               $0.name.caseInsensitiveCompare(defaultTransferDeviceName) == .orderedSame
                           }) {
                            Text("\(defaultTransferDeviceName) — unavailable")
                                .tag(defaultTransferDeviceName)
                        }
                    }
                    .onChange(of: defaultTransferDeviceName) { _, name in
                        store.defaultTransferDeviceName = name
                    }

                    Text("Choose Ask Every Time to pick a device in the share sheet. A saved destination sends files immediately. Text still updates the shared clipboard on every device.")
                        .font(.footnote)
                        .foregroundStyle(.secondary)

                    Text("Files sent from your Mac are accepted automatically while Tailboard is open and saved in Files → On My iPhone → Tailboard.")
                        .font(.footnote)
                        .foregroundStyle(.secondary)
                }

                Section("Control Center") {
                    LabeledContent("Configured Controls") {
                        Text("\(store.configuredControlKinds.count)")
                    }
                    if let diagnostic = controlDiagnostic {
                        LabeledContent("Last Action", value: diagnostic.action)
                        LabeledContent("Result", value: diagnostic.status)
                        if !diagnostic.detail.isEmpty {
                            Text(diagnostic.detail)
                                .font(.footnote)
                                .foregroundStyle(
                                    diagnostic.status == "Failed" ? Color.red : Color.secondary
                                )
                        }
                        Text(diagnostic.updatedAt.formatted(date: .omitted, time: .standard))
                            .font(.caption)
                            .foregroundStyle(.secondary)
                    } else {
                        Text("No Control Center action has run yet.")
                            .foregroundStyle(.secondary)
                    }

                    Button("Refresh Control Diagnostics") {
                        controlDiagnostic = store.lastControlDiagnostic
                    }
                }

                // The only place plumbing is visible (spec §3): the sync
                // server is an implementation detail kept here for
                // debugging, not part of the user's mental model.
                Section("Advanced") {
                    LabeledContent("Sync Server") {
                        Text(store.hubURL?.absoluteString ?? "Not set")
                            .font(.caption.monospaced())
                            .foregroundStyle(.secondary)
                            .textSelection(.enabled)
                    }

                    // Not destructive: nothing is cleared unless a new
                    // probe succeeds, and the sheet can always be cancelled.
                    Button("Change Sync Server…") {
                        viewModel.beginReconfigure()
                    }
                }

                Section("About") {
                    LabeledContent("Version", value: Bundle.main.infoDictionary?["CFBundleShortVersionString"] as? String ?? "1.0")
                }
            }
            .navigationTitle("Settings")
            .onAppear {
                controlDiagnostic = store.lastControlDiagnostic
                defaultTransferDeviceName = store.defaultTransferDeviceName
                Task { await viewModel.refresh() }
            }
        }
    }

    private func commitRename() {
        let name = deviceName.trimmingCharacters(in: .whitespacesAndNewlines)
        guard !name.isEmpty else {
            deviceName = store.sourceName
            return
        }
        Task { await viewModel.renameDevice(to: name) }
    }

    private var statusColor: Color {
        switch viewModel.connectionState {
        case .connected: return .green
        case .connecting: return .orange
        case .disconnected: return .orange
        case .noVPN: return .red
        }
    }

    private var transferDevices: [Device] {
        viewModel.devices
            .filter { $0.deviceID != store.deviceID && $0.capabilities.contains("transfers") }
            .sorted { ($0.online ? 0 : 1, $0.name) < ($1.online ? 0 : 1, $1.name) }
    }
}
