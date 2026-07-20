import SwiftUI
import UIKit
import ClipHubKit

struct SettingsView: View {
    @Environment(AppViewModel.self) private var viewModel
    @Environment(\.openURL) private var openURL
    private let store = AppGroupStore()

    var body: some View {
        NavigationStack {
            Form {
                Section("Connection") {
                    LabeledContent("Status") {
                        StatusDot(color: statusColor, text: viewModel.connectionState.label)
                    }

                    LabeledContent("This Device") {
                        Text(store.sourceName)
                            .foregroundStyle(.secondary)
                    }

                    Button("View Devices") {
                        viewModel.selectedTab = .devices
                    }
                }

                Section("Keyboard Extension") {
                    Text("Enable Tail Paste in Settings → General → Keyboard → Keyboards → Add New Keyboard → ClipHub.")
                        .font(.footnote)
                        .foregroundStyle(.secondary)

                    Button("Open Settings") {
                        guard let url = URL(string: UIApplication.openSettingsURLString) else { return }
                        openURL(url)
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
        }
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
