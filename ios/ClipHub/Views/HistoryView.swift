import SwiftUI
import ClipHubKit

/// Full clipboard history. Pushed from the Clipboard home "Show All" row,
/// so it intentionally has no NavigationStack of its own.
struct HistoryView: View {
    @Environment(AppViewModel.self) private var viewModel
    @Environment(\.accessibilityReduceMotion) private var reduceMotion

    var body: some View {
        List(viewModel.history) { item in
            ClipRowView(item: item, copy: viewModel.copyToPasteboard)
        }
        .animation(reduceMotion ? .none : .default, value: viewModel.history)
        .navigationTitle("History")
        .refreshable { await viewModel.refresh() }
        .overlay {
            if viewModel.history.isEmpty {
                ContentUnavailableView(
                    "Nothing here yet",
                    systemImage: "clock",
                    description: Text("Everything you copy on your devices shows up here.")
                )
            }
        }
        // Row copy failures (unsupported binary clips) surface here, since
        // the confirmation is deliberately gated on the copy succeeding.
        .overlay(alignment: .top) {
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
        .animation(reduceMotion ? .none : .clipHub, value: viewModel.errorMessage != nil)
    }
}
