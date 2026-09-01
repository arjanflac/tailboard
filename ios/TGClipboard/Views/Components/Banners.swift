import SwiftUI
import TGClipboardKit

/// Shared banner background: translucent material by default, solid
/// adaptive surface when Reduce Transparency is on.
private struct BannerMaterial: ViewModifier {
    var cornerRadius: CGFloat = 12
    @Environment(\.accessibilityReduceTransparency) private var reduceTransparency

    func body(content: Content) -> some View {
        content.background {
            RoundedRectangle(cornerRadius: cornerRadius, style: .continuous)
                .fill(
                    reduceTransparency
                        ? AnyShapeStyle(Color(.secondarySystemBackground))
                        : AnyShapeStyle(.regularMaterial)
                )
        }
    }
}

private extension View {
    func bannerMaterial(cornerRadius: CGFloat = 12) -> some View {
        modifier(BannerMaterial(cornerRadius: cornerRadius))
    }
}

/// Inline connection status shown only while not connected. Icon + text,
/// so meaning never depends on color alone.
struct ConnectionBanner: View {
    let state: ConnectionState

    @Environment(\.openURL) private var openURL

    private var tint: Color {
        switch state {
        case .connected: return .green
        case .connecting: return .orange
        case .disconnected: return .orange
        case .noVPN: return .red
        }
    }

    private var icon: String {
        switch state {
        case .connected: return "checkmark.circle.fill"
        case .connecting: return "antenna.radiowaves.left.and.right"
        case .disconnected: return "wifi.exclamationmark"
        case .noVPN: return "network.slash"
        }
    }

    var body: some View {
        HStack(spacing: 8) {
            HStack(spacing: 8) {
                Image(systemName: icon)
                    .foregroundStyle(tint)
                Text(state.label)
                    .font(.footnote.weight(.medium))
                    .foregroundStyle(.primary)
                    .lineLimit(2)
            }
            .accessibilityElement(children: .combine)
            .accessibilityLabel("Connection status: \(state.label)")
            Spacer(minLength: 0)
            if state == .noVPN {
                Button("Open Tailscale") {
                    openTailscale()
                }
                .font(.footnote.weight(.semibold))
                .buttonStyle(.bordered)
                .controlSize(.small)
            }
        }
        .padding(.horizontal, 12)
        .padding(.vertical, 8)
        .bannerMaterial()
    }

    /// Deep-links into Tailscale when installed, otherwise offers the App
    /// Store page (Tailscale app id 891049268).
    private func openTailscale() {
        if let deepLink = URL(string: "tailscale://"),
           UIApplication.shared.canOpenURL(deepLink) {
            openURL(deepLink)
        } else if let appStore = URL(string: "https://apps.apple.com/app/id891049268") {
            openURL(appStore)
        }
    }
}

/// Transient error surface for failed refresh or clipboard actions.
/// Previously these errors were written to the view model and never shown.
struct ErrorBanner: View {
    let message: String
    let dismiss: () -> Void

    var body: some View {
        HStack(spacing: 8) {
            Image(systemName: "exclamationmark.triangle.fill")
                .foregroundStyle(.red)
            Text(message)
                .font(.footnote)
                .foregroundStyle(.primary)
                .lineLimit(2)
            Spacer(minLength: 0)
            Button(action: dismiss) {
                Image(systemName: "xmark")
                    .font(.caption.weight(.semibold))
                    .foregroundStyle(.secondary)
            }
            .accessibilityLabel("Dismiss error")
        }
        .padding(.horizontal, 12)
        .padding(.vertical, 8)
        .bannerMaterial()
    }
}

/// Confirmation pill shown when a clip is copied via the widget deep link,
/// so a cold-open copy is never silent.
struct CopiedPill: View {
    var body: some View {
        Label("Copied to iPhone clipboard", systemImage: "checkmark.circle.fill")
            .font(.footnote.weight(.semibold))
            .foregroundStyle(.primary)
            .padding(.horizontal, 14)
            .padding(.vertical, 8)
            .bannerMaterial(cornerRadius: 999)
    }
}

struct ReceivedPill: View {
    let message: String

    var body: some View {
        Label(message, systemImage: "checkmark.circle.fill")
            .font(.footnote.weight(.semibold))
            .foregroundStyle(.primary)
            .padding(.horizontal, 14)
            .padding(.vertical, 8)
            .bannerMaterial(cornerRadius: 999)
    }
}
