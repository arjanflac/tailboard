import SwiftUI
import TGClipboardKit

struct ShareExtensionView: View {
    @Bindable var viewModel: ShareExtensionViewModel
    let cancel: () -> Void
    let complete: () -> Void

    var body: some View {
        VStack(alignment: .leading, spacing: 14) {
            HStack {
                Image(systemName: "doc.on.clipboard")
                    .font(.title2)
                VStack(alignment: .leading, spacing: 2) {
                    Text("Send with Tailboard")
                        .font(.headline)
                    Text(fileSummary)
                        .font(.caption)
                        .foregroundStyle(.secondary)
                }
                Spacer()
            }

            Divider()

            switch viewModel.state {
            case .loading:
                progress("Finding your devices…")
            case .ready:
                deviceButtons
            case .sending(let name):
                progress("Sending to \(name)…")
            case .sent(let name):
                Label("Sent to \(name)", systemImage: "checkmark.circle.fill")
                    .foregroundStyle(.green)
                    .frame(maxWidth: .infinity, minHeight: 90)
                    .task {
                        try? await Task.sleep(for: .milliseconds(700))
                        complete()
                    }
            case .failed(let message):
                VStack(alignment: .leading, spacing: 10) {
                    Label(message, systemImage: "exclamationmark.triangle.fill")
                        .foregroundStyle(.orange)
                    Button("Close", action: cancel)
                        .keyboardShortcut(.cancelAction)
                }
                .frame(maxWidth: .infinity, minHeight: 90, alignment: .leading)
            }
        }
        .padding(18)
        .frame(width: 380, height: 250)
    }

    private var deviceButtons: some View {
        VStack(spacing: 8) {
            ForEach(viewModel.devices) { device in
                Button {
                    Task { await viewModel.send(to: device) }
                } label: {
                    HStack {
                        Image(systemName: device.platformSymbol)
                            .frame(width: 26)
                        Text(viewModel.friendlyName(device))
                            .fontWeight(.medium)
                        Spacer()
                        Circle()
                            .fill(device.online ? Color.green : Color.gray)
                            .frame(width: 8, height: 8)
                        Text(device.online ? "Online" : "Queued")
                            .font(.caption)
                            .foregroundStyle(.secondary)
                    }
                    .padding(.horizontal, 10)
                    .frame(maxWidth: .infinity, minHeight: 38)
                }
                .buttonStyle(.bordered)
            }
            Button("Cancel", action: cancel)
                .buttonStyle(.plain)
                .font(.caption)
                .foregroundStyle(.secondary)
                .keyboardShortcut(.cancelAction)
        }
    }

    private func progress(_ text: String) -> some View {
        HStack(spacing: 10) {
            ProgressView()
            Text(text)
                .foregroundStyle(.secondary)
        }
        .frame(maxWidth: .infinity, minHeight: 90)
    }

    private var fileSummary: String {
        let count = viewModel.files.count
        return count == 1 ? viewModel.files[0].lastPathComponent : "\(count) files"
    }
}
