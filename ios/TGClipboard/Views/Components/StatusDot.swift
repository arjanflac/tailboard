import SwiftUI

/// A status indicator that is never color-only: the dot is decorative,
/// the text carries the meaning (VoiceOver reads the combined element).
struct StatusDot: View {
    let color: Color
    let text: String

    var body: some View {
        HStack(spacing: 6) {
            Circle()
                .fill(color)
                .frame(width: 8, height: 8)
                .accessibilityHidden(true)
            Text(text)
                .font(.footnote)
                .foregroundStyle(.secondary)
        }
        .accessibilityElement(children: .combine)
    }
}
