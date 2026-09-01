import SwiftUI
import UIKit

struct ShareView: View {
    @Bindable var viewModel: ShareViewModel
    let onDismiss: () -> Void

    @Environment(\.accessibilityReduceMotion) private var reduceMotion
    @State private var successTick = 0
    @State private var dismissTask: Task<Void, Never>?

    var body: some View {
        NavigationStack {
            VStack(spacing: 18) {
                switch viewModel.state {
                case .extracting, .sending:
                    ProgressView(viewModel.state == .extracting ? "Preparing…" : "Sending clipboard…")
                        .frame(maxHeight: .infinity)
                case .sent:
                    successView
                case .ready, .failed:
                    formView
                }
            }
            .padding()
            .frame(maxWidth: .infinity, maxHeight: .infinity, alignment: .top)
            .navigationTitle("Tailboard")
            .navigationBarTitleDisplayMode(.inline)
            .toolbar {
                ToolbarItem(placement: .cancellationAction) {
                    Button("Cancel", action: onDismiss)
                }
            }
        }
        .onChange(of: viewModel.state) { _, state in
            guard state == .sent else { return }
            successTick += 1
            UIAccessibility.post(notification: .announcement, argument: "Clipboard sent")
            dismissTask?.cancel()
            dismissTask = Task { @MainActor in
                try? await Task.sleep(for: .milliseconds(800))
                guard !Task.isCancelled else { return }
                onDismiss()
            }
        }
        .onDisappear { dismissTask?.cancel() }
        .sensoryFeedback(.success, trigger: successTick)
        .animation(reduceMotion ? .none : .easeInOut(duration: 0.2), value: viewModel.state)
    }

    private var formView: some View {
        VStack(spacing: 14) {
            if viewModel.hasContent {
                Text(viewModel.textToSend)
                    .font(.callout)
                    .lineLimit(8)
                    .frame(maxWidth: .infinity, alignment: .leading)
                    .padding(12)
                    .background(.fill.secondary, in: .rect(cornerRadius: 12))
            }
            if case .failed(let message) = viewModel.state {
                Label(message, systemImage: "exclamationmark.triangle.fill")
                    .font(.footnote)
                    .foregroundStyle(.secondary)
            }
            if viewModel.hasContent {
                Button("Try Again") { Task { await viewModel.send() } }
                    .buttonStyle(.borderedProminent)
            }
        }
    }

    private var successView: some View {
        VStack(spacing: 12) {
            Spacer()
            Image(systemName: "checkmark.circle.fill")
                .font(.system(size: 56))
                .foregroundStyle(.green)
                .symbolEffect(.bounce, value: reduceMotion ? 0 : successTick)
            Text("Clipboard sent")
                .font(.headline)
            Text("Available on your Tailboard devices")
                .font(.caption)
                .foregroundStyle(.secondary)
            Spacer()
        }
    }
}
