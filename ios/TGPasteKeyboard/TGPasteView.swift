import SwiftUI
import TGClipboardKit

/// Press feedback for the recent-clip chips: instant response on touch-down,
/// critically damped (no bounce) and reduced to a plain opacity change when
/// Reduce Motion is on.
private struct ChipButtonStyle: ButtonStyle {
    @Environment(\.accessibilityReduceMotion) private var reduceMotion

    func makeBody(configuration: Configuration) -> some View {
        configuration.label
            .font(.caption.weight(.medium))
            .lineLimit(1)
            .padding(.horizontal, 12)
            .frame(minHeight: 36)
            .background(.fill.tertiary, in: .rect(cornerRadius: 8, style: .continuous))
            .opacity(configuration.isPressed ? 0.65 : 1)
            .scaleEffect(!reduceMotion && configuration.isPressed ? 0.96 : 1)
            .animation(.spring(response: 0.25, dampingFraction: 1.0), value: configuration.isPressed)
    }
}

struct TGPasteView: View {
    @Bindable var viewModel: KeyboardViewModel
    /// Globe button is only shown when the system says the user has other
    /// keyboards to switch to (UIInputViewController.needsInputModeSwitchKey).
    var showsGlobe: Bool
    var onGlobe: () -> Void

    @Environment(\.accessibilityReduceMotion) private var reduceMotion

    var body: some View {
        VStack(spacing: 8) {
            primaryRow
            stripRow
            statusRow
        }
        .padding(.horizontal, 8)
        .padding(.vertical, 6)
    }

    // MARK: - Primary row: globe / paste / push

    private var primaryRow: some View {
        HStack(spacing: 8) {
            if showsGlobe {
                Button(action: onGlobe) {
                    Image(systemName: "globe")
                        .frame(width: 40, height: 44)
                }
                .buttonStyle(.bordered)
                .accessibilityLabel("Next keyboard")
                .accessibilityHint("Switches to the next enabled keyboard")
            }

            Button(action: viewModel.insertCurrent) {
                HStack(spacing: 6) {
                    Image(systemName: "doc.on.clipboard.fill")
                    if let clip = viewModel.currentClip {
                        Text(clip.preview)
                            .lineLimit(1)
                            .truncationMode(.tail)
                    } else {
                        Text("Tailboard")
                    }
                }
                .font(.body.weight(.medium))
                .frame(maxWidth: .infinity)
                .frame(height: 44)
            }
            .buttonStyle(.borderedProminent)
            .disabled(viewModel.currentClip == nil)
            .accessibilityLabel(viewModel.currentClip == nil ? "Tailboard, nothing to insert" : "Paste current clip")
            .accessibilityHint("Inserts the most recent Tailboard clip into the current text field")

            Button(action: viewModel.pushClipboard) {
                Image(systemName: "arrow.up.doc.fill")
                    .frame(width: 40, height: 44)
            }
            .buttonStyle(.bordered)
            .accessibilityLabel("Send clipboard")
            .accessibilityHint("Sends this device's clipboard to your other devices")
        }
    }

    // MARK: - Recent clips strip (always present; holds the empty hint)

    @ViewBuilder
    private var stripRow: some View {
        if let hint = viewModel.setupHint {
            stripHint(hint, systemImage: "exclamationmark.triangle")
        } else if viewModel.textClips.isEmpty && viewModel.imageClip == nil {
            stripHint("Copy something on another device — it appears here.", systemImage: "doc.on.clipboard")
        } else {
            ScrollView(.horizontal, showsIndicators: false) {
                HStack(spacing: 8) {
                    if let imageClip = viewModel.imageClip {
                        Button {
                            viewModel.copyImageClip(imageClip)
                        } label: {
                            Label(imageClip.displaySummary, systemImage: "photo")
                        }
                        .buttonStyle(ChipButtonStyle())
                        .accessibilityLabel("Copy image clip")
                        .accessibilityHint("Copies the image so you can paste it into the app")
                    }
                    ForEach(Array(viewModel.textClips.prefix(5).enumerated()), id: \.element.id) { index, clip in
                        Button {
                            viewModel.insert(clip)
                        } label: {
                            Text(clip.preview)
                        }
                        .buttonStyle(ChipButtonStyle())
                        .accessibilityLabel("Insert clip \(index + 1)")
                        .accessibilityValue(clip.preview)
                        .accessibilityHint("Inserts this text into the current field")
                    }
                }
                .padding(.horizontal, 4)
            }
            .frame(height: 36)
        }
    }

    private func stripHint(_ text: String, systemImage: String) -> some View {
        HStack(spacing: 6) {
            Image(systemName: systemImage)
            Text(text)
        }
        .font(.caption)
        .foregroundStyle(.secondary)
        .frame(maxWidth: .infinity)
        .frame(height: 36)
    }

    // MARK: - Typed transient feedback row

    private var statusRow: some View {
        HStack(spacing: 6) {
            switch viewModel.status {
            case .idle:
                EmptyView()
            case .working(let message):
                ProgressView()
                    .controlSize(.small)
                Text(message)
                    .font(.caption)
                    .foregroundStyle(.secondary)
            case .success(let message):
                Image(systemName: "checkmark.circle.fill")
                    .foregroundStyle(.green)
                Text(message)
                    .font(.caption)
                    .foregroundStyle(.secondary)
            case .failure(let message):
                Image(systemName: "exclamationmark.triangle.fill")
                    .foregroundStyle(.orange)
                Text(message)
                    .font(.caption)
                    .foregroundStyle(.secondary)
            }
            Spacer()
        }
        .padding(.horizontal, 4)
        .frame(height: 20)
        .contentTransition(.opacity)
        // Under Reduce Motion the status swaps instantly instead of fading.
        .animation(reduceMotion ? nil : .easeInOut(duration: 0.2), value: viewModel.status)
        .accessibilityElement(children: .combine)
    }
}
