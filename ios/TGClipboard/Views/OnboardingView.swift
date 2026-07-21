import SwiftUI
import TGClipboardKit

struct OnboardingView: View {
    /// First run passes false (no way past setup); the reconfiguration
    /// sheet passes true so the user can always cancel back into the app.
    let allowsCancel: Bool

    @Environment(AppViewModel.self) private var viewModel
    @Environment(\.dismiss) private var dismiss

    @State private var hubURLText = ""
    @State private var sourceName = ""
    @State private var isProbing = false
    @State private var probeError: String?
    @State private var showInvalidURL = false

    var body: some View {
        NavigationStack {
            VStack(spacing: 24) {
                if allowsCancel {
                    HStack {
                        Spacer()
                        Button("Cancel") { dismiss() }
                    }
                    .padding(.horizontal)
                }

                Spacer()

                Image(systemName: "doc.on.clipboard.fill")
                    .font(.system(size: 64))
                    .foregroundStyle(.tint)
                    .accessibilityHidden(true)

                Text("tg-clipboard")
                    .font(.largeTitle.bold())

                Text("Your clipboard and files, on every device. Private, over Tailscale.")
                    .font(.body)
                    .foregroundStyle(.secondary)
                    .multilineTextAlignment(.center)
                    .padding(.horizontal)

                Spacer()

                VStack(alignment: .leading, spacing: 12) {
                    Text("Sync Server Address")
                        .font(.headline)
                    TextField("http://100.x.x.x", text: $hubURLText)
                        .textFieldStyle(.roundedBorder)
                        .autocorrectionDisabled()
                        .textInputAutocapitalization(.never)
                        .keyboardType(.URL)
                        .submitLabel(.go)
                        .onSubmit(connect)
                        .onChange(of: hubURLText) { showInvalidURL = false }

                    if showInvalidURL {
                        Label(
                            "Enter a valid address, for example http://100.64.1.2",
                            systemImage: "exclamationmark.triangle.fill"
                        )
                        .font(.caption)
                        .foregroundStyle(.red)
                    } else {
                        Text("The address of the device keeping your other devices in sync.")
                            .font(.caption)
                            .foregroundStyle(.secondary)
                    }

                    Text("Device Name")
                        .font(.headline)
                    TextField("iPhone", text: $sourceName)
                        .textFieldStyle(.roundedBorder)
                        .submitLabel(.go)
                        .onSubmit(connect)

                    if let error = probeError {
                        Label(error, systemImage: "exclamationmark.triangle.fill")
                            .font(.caption)
                            .foregroundStyle(.red)
                    }
                }
                .padding(.horizontal)

                Button(action: connect) {
                    if isProbing {
                        ProgressView()
                            .frame(maxWidth: .infinity)
                    } else {
                        Text("Connect")
                            .frame(maxWidth: .infinity)
                    }
                }
                .buttonStyle(.borderedProminent)
                .controlSize(.large)
                .disabled(isProbing)
                .padding(.horizontal)

                Spacer()
            }
            .navigationBarHidden(true)
            .onAppear {
                if hubURLText.isEmpty, let existing = AppGroupStore().hubURL {
                    hubURLText = existing.absoluteString
                }
                if sourceName.isEmpty {
                    sourceName = AppGroupStore().sourceName
                }
            }
        }
    }

    private func connect() {
        guard !hubURLText.isEmpty,
              let url = URL(string: hubURLText),
              url.scheme != nil, url.host != nil else {
            showInvalidURL = true
            return
        }
        showInvalidURL = false
        isProbing = true
        probeError = nil

        Task {
            let name = sourceName.isEmpty ? "iphone" : sourceName
            let ok = await viewModel.configureHub(url: url, sourceName: name)
            isProbing = false
            if !ok {
                probeError = "Can't reach your sync server. Is Tailscale on?"
            }
            // On success the view model flips showOnboarding / isReconfiguring,
            // which dismisses the full-screen flow or the sheet automatically.
        }
    }
}
