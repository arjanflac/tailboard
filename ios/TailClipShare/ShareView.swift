import SwiftUI
import UIKit
import ClipHubKit

struct ShareView: View {
    @Bindable var viewModel: ShareViewModel
    let onDismiss: () -> Void

    @Environment(\.accessibilityReduceMotion) private var reduceMotion
    @State private var successTick = 0
    @State private var dismissTask: Task<Void, Never>?

    var body: some View {
        NavigationStack {
            VStack(spacing: 20) {
                switch viewModel.state {
                case .extracting:
                    extractingView
                        .transition(.opacity)
                case .sent:
                    successView
                        .transition(reduceMotion ? .opacity : .opacity.combined(with: .scale(scale: 0.96)))
                default:
                    formView
                        .transition(.opacity)
                }
            }
            .padding()
            .frame(maxWidth: .infinity, maxHeight: .infinity, alignment: .top)
            .animation(
                reduceMotion ? .easeInOut(duration: 0.2) : .spring(response: 0.35, dampingFraction: 1.0),
                value: viewModel.state
            )
            .navigationTitle("ClipHub")
            .navigationBarTitleDisplayMode(.inline)
            .toolbar {
                ToolbarItem(placement: .cancellationAction) {
                    Button("Cancel", action: onDismiss)
                }
                if viewModel.state == .sent {
                    ToolbarItem(placement: .confirmationAction) {
                        Button("Done", action: onDismiss)
                    }
                }
            }
        }
        .task { await viewModel.loadDevices() }
        .onChange(of: viewModel.state) { _, newState in
            handleStateChange(newState)
        }
        .onDisappear {
            dismissTask?.cancel()
        }
        .sensoryFeedback(.success, trigger: successTick)
    }

    // MARK: - State: extracting

    private var extractingView: some View {
        VStack(spacing: 12) {
            Spacer(minLength: 40)
            ProgressView()
            Text("Preparing…")
                .font(.callout)
                .foregroundStyle(.secondary)
            Spacer()
        }
        .frame(maxWidth: .infinity)
        .accessibilityElement(children: .combine)
        .accessibilityLabel("Preparing shared content")
    }

    // MARK: - State: ready / sending / failed

    private var formView: some View {
        VStack(spacing: 16) {
            if viewModel.hasContent {
                previewCard
            } else {
                ContentUnavailableView(
                    "Nothing to Send",
                    systemImage: "doc.questionmark",
                    description: Text("This content can't be sent with ClipHub.")
                )
            }

            if viewModel.hasContent {
                destinationPicker
            }

            if case .failed(let message) = viewModel.state {
                HStack(alignment: .top, spacing: 8) {
                    Image(systemName: "exclamationmark.triangle.fill")
                        .foregroundStyle(.orange)
                    Text(message)
                        .font(.caption)
                        .foregroundStyle(.secondary)
                        .frame(maxWidth: .infinity, alignment: .leading)
                }
                .padding(10)
                .background(.fill.secondary, in: .rect(cornerRadius: 10, style: .continuous))
                .accessibilityElement(children: .combine)
                .transition(.opacity)
            }

            sendButton
        }
    }

    private var previewCard: some View {
        VStack(alignment: .leading, spacing: 8) {
            if let thumbnail = viewModel.thumbnail {
                Image(uiImage: thumbnail)
                    .resizable()
                    .scaledToFit()
                    .frame(maxHeight: 160)
                    .clipShape(.rect(cornerRadius: 8, style: .continuous))
                    .accessibilityLabel("Shared image preview")
            } else if !viewModel.previewText.isEmpty {
                Text(viewModel.previewText)
                    .font(.callout)
                    .lineLimit(4)
                    .multilineTextAlignment(.leading)
                    .frame(maxWidth: .infinity, alignment: .leading)
                    .textSelection(.enabled)
            } else {
                Label(viewModel.contentSummary, systemImage: "doc")
                    .font(.callout)
            }

            HStack(alignment: .firstTextBaseline) {
                Text(viewModel.suggestedName)
                    .font(.caption.weight(.medium))
                    .lineLimit(1)
                    .truncationMode(.middle)
                Spacer(minLength: 8)
                Text(viewModel.contentSummary)
                    .font(.caption)
                    .foregroundStyle(.secondary)
            }
        }
        .padding(12)
        .frame(maxWidth: .infinity, alignment: .leading)
        .background(.fill.secondary, in: .rect(cornerRadius: 12, style: .continuous))
    }

    private var destinationPicker: some View {
        Picker("Send to", selection: $viewModel.selectedDeviceID) {
            Label("All my devices (clipboard)", systemImage: "doc.on.clipboard")
                .tag("")
            ForEach(viewModel.devices) { device in
                Label {
                    Text("\(device.name) — \(device.statusLabel)")
                } icon: {
                    Image(systemName: device.platformSymbol)
                }
                .tag(device.deviceID)
            }
        }
        .pickerStyle(.menu)
        .disabled(viewModel.state == .sending)
        .accessibilityHint("Chooses where the content is sent")
    }

    private var sendButton: some View {
        Button {
            Task { await viewModel.send() }
        } label: {
            Group {
                if viewModel.state == .sending {
                    ProgressView()
                } else {
                    Label("Send to \(viewModel.destinationName)", systemImage: "paperplane.fill")
                }
            }
            .frame(maxWidth: .infinity)
            .frame(height: 44) // fixed height: no layout jump when the spinner appears
        }
        .buttonStyle(.borderedProminent)
        .disabled(viewModel.state == .sending || !viewModel.hasContent)
        .accessibilityHint("Sends the content, then dismisses the sheet")
    }

    // MARK: - State: sent

    private var successView: some View {
        VStack(spacing: 12) {
            Spacer(minLength: 24)
            Image(systemName: "checkmark.circle.fill")
                .font(.system(size: 56))
                .foregroundStyle(.green)
                // Tick is frozen under Reduce Motion, so no bounce fires.
                .symbolEffect(.bounce, value: reduceMotion ? 0 : successTick)
                .accessibilityLabel("Sent")
            Text("Sent to \(viewModel.destinationName)")
                .font(.headline)
            Text("Tap to stay")
                .font(.caption)
                .foregroundStyle(.tertiary)
            Spacer()
        }
        .frame(maxWidth: .infinity)
        .contentShape(Rectangle())
        .onTapGesture { dismissTask?.cancel() }
        .accessibilityHint("Dismisses automatically. Tap to keep this sheet open.")
    }

    // MARK: - State side effects

    private func handleStateChange(_ newState: SendState) {
        switch newState {
        case .sent:
            successTick += 1
            UIAccessibility.post(notification: .announcement, argument: "Sent to \(viewModel.destinationName)")
            dismissTask?.cancel()
            dismissTask = Task { @MainActor in
                try? await Task.sleep(for: .milliseconds(900))
                guard !Task.isCancelled else { return }
                onDismiss()
            }
        case .failed(let message):
            UIAccessibility.post(notification: .announcement, argument: message)
        default:
            break
        }
    }
}
