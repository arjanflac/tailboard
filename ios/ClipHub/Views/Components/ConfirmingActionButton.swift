import SwiftUI

/// Button that confirms a completed action by morphing its label to a
/// checkmark state ("Copy to iPhone" → "Copied") with a symbol bounce and a
/// success haptic, then reverts. The confirmation fires only after the
/// action reports success, so feedback stays causal. Under Reduce Motion the
/// morph is a plain cross-fade (no bounce, no spring).
/// Compact circular secondary action (Send, etc.) with the same causal
/// ✓ confirmation behavior as ConfirmingActionButton.
struct CircleActionButton: View {
    let systemImage: String
    let confirmedIcon: String
    var action: () async -> Void

    @State private var confirmed = false
    @State private var running = false
    @Environment(\.accessibilityReduceMotion) private var reduceMotion

    var body: some View {
        Button {
            guard !running else { return }
            running = true
            Task {
                await action()
                running = false
                withAnimation(reduceMotion ? .none : .clipHub) { confirmed = true }
                try? await Task.sleep(for: .seconds(1.6))
                withAnimation(reduceMotion ? .none : .clipHub) { confirmed = false }
            }
        } label: {
            Image(systemName: confirmed ? confirmedIcon : systemImage)
                .font(.body.weight(.semibold))
                .frame(width: 44, height: 44)
                .symbolEffect(.bounce, value: confirmed)
        }
        .buttonStyle(.bordered)
        .clipShape(Circle())
        .sensoryFeedback(.success, trigger: confirmed) { _, new in new }
    }
}

struct ConfirmingActionButton: View {
    let title: String
    let confirmedTitle: String
    let systemImage: String
    var prominent = false
    /// Return true when the action succeeded; the confirmation shows only then.
    var action: () async -> Bool

    @State private var confirmed = false
    @State private var running = false
    @State private var revertTask: Task<Void, Never>?
    @Environment(\.accessibilityReduceMotion) private var reduceMotion

    var body: some View {
        button
            .sensoryFeedback(.success, trigger: confirmed) { _, new in new }
    }

    @ViewBuilder
    private var button: some View {
        if prominent {
            base.buttonStyle(.borderedProminent)
        } else {
            base.buttonStyle(.bordered)
        }
    }

    private var base: some View {
        Button {
            guard !running, !confirmed else { return }
            running = true
            Task {
                let ok = await action()
                running = false
                guard ok else { return }
                withAnimation(reduceMotion ? .none : .clipHub) {
                    confirmed = true
                }
                revertTask?.cancel()
                revertTask = Task { @MainActor in
                    try? await Task.sleep(for: .seconds(2))
                    guard !Task.isCancelled else { return }
                    withAnimation(reduceMotion ? .none : .clipHub) {
                        confirmed = false
                    }
                }
            }
        } label: {
            Label(
                confirmed ? confirmedTitle : title,
                systemImage: confirmed ? "checkmark" : systemImage
            )
            .symbolEffect(.bounce, value: confirmed)
            .frame(maxWidth: prominent ? .infinity : nil)
        }
    }
}
