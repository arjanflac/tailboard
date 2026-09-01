import SwiftUI
import TGClipboardKit

struct SettingsView: View {
    @Environment(AppViewModel.self) private var viewModel
    private let store = AppGroupStore()
    @State private var deviceName = AppGroupStore().sourceName
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

                    LabeledContent("Recent Clips", value: "20 items · 24 hours")
                }

                Section("About") {
                    LabeledContent("Version", value: Bundle.main.infoDictionary?["CFBundleShortVersionString"] as? String ?? "1.0")
                }
            }
            .navigationTitle("Settings")
            .onAppear { Task { await viewModel.refresh() } }
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
}
