import SwiftUI
import TGClipboardKit

struct PopoverView: View {
    @Environment(MenuBarViewModel.self) private var viewModel

    var body: some View {
        VStack(alignment: .leading, spacing: 14) {
            if viewModel.status == .engineOff {
                engineOffCard
            } else if viewModel.status == .noTailscale {
                noTailscaleCard
            } else {
                if let clip = viewModel.state?.clip {
                    ClipCard(clip: clip, size: viewModel.state?.clipSize ?? 0)
                }
                devicesSection
            }
            if let message = viewModel.errorMessage {
                errorRow(message)
            }
            footer
        }
        .padding(14)
        .frame(width: 320)
        .task { await viewModel.refresh() }
    }

    private var engineOffCard: some View {
        VStack(spacing: 10) {
            Image(systemName: "powerplug")
                .font(.system(size: 28))
                .foregroundStyle(.secondary)
            Text("Tailboard Engine isn't running")
                .font(.headline)
            Text("Start it to sync your clipboard.")
                .font(.caption)
                .foregroundStyle(.secondary)
            Button("Start") { viewModel.startEngine() }
                .buttonStyle(.borderedProminent)
        }
        .frame(maxWidth: .infinity)
        .padding(.vertical, 20)
    }

    private var noTailscaleCard: some View {
        VStack(spacing: 10) {
            Image(systemName: "network.slash")
                .font(.system(size: 28))
                .foregroundStyle(.secondary)
            Text("Tailscale is off")
                .font(.headline)
            Text("Your devices reach each other over Tailscale.")
                .font(.caption)
                .foregroundStyle(.secondary)
            Button("Open Tailscale") { viewModel.openTailscale() }
                .buttonStyle(.borderedProminent)
        }
        .frame(maxWidth: .infinity)
        .padding(.vertical, 20)
    }

    private var devicesSection: some View {
        VStack(alignment: .leading, spacing: 8) {
            Text("Devices")
                .font(.caption.weight(.semibold))
                .foregroundStyle(.secondary)
                .textCase(.uppercase)

            if viewModel.otherDevices.isEmpty {
                Text("No devices yet")
                    .font(.callout)
                    .foregroundStyle(.secondary)
                    .frame(maxWidth: .infinity)
                    .padding(.vertical, 14)
            } else {
                LazyVGrid(columns: [GridItem(.adaptive(minimum: 90), spacing: 8)], spacing: 8) {
                    ForEach(viewModel.otherDevices) { device in
                        DeviceStatusTile(device: device)
                    }
                }
            }
        }
    }

    private func errorRow(_ message: String) -> some View {
        HStack(spacing: 6) {
            Image(systemName: "exclamationmark.triangle.fill")
                .foregroundStyle(.orange)
            Text(message).font(.caption).lineLimit(2)
            Spacer(minLength: 0)
            Button { viewModel.errorMessage = nil } label: {
                Image(systemName: "xmark").font(.caption2)
            }
            .buttonStyle(.plain)
        }
        .padding(8)
        .background(.quaternary.opacity(0.5), in: RoundedRectangle(cornerRadius: 8))
    }

    private var footer: some View {
        HStack {
            Text(viewModel.statusLine)
                .font(.caption)
                .foregroundStyle(.secondary)
            Spacer()
            if viewModel.status != .engineOff {
                Button { viewModel.togglePaused() } label: {
                    Image(systemName: viewModel.status == .paused ? "play.fill" : "pause.fill")
                }
                .buttonStyle(.plain)
                .foregroundStyle(.secondary)
                .help(viewModel.status == .paused ? "Resume clipboard sync" : "Pause clipboard sync")
            }
            Button { NSApp.terminate(nil) } label: {
                Image(systemName: "power")
            }
            .buttonStyle(.plain)
            .foregroundStyle(.secondary)
            .help("Quit Tailboard")
        }
    }
}

private struct ClipCard: View {
    @Environment(MenuBarViewModel.self) private var viewModel
    let clip: ClipItem
    let size: Int64

    private var summary: String {
        clip.isText ? clip.displaySummary
            : ClipItem.displaySummary(mimeType: clip.mimeType, byteCount: Int(size), textContent: nil)
    }

    var body: some View {
        VStack(alignment: .leading, spacing: 6) {
            HStack {
                Label(summary, systemImage: clip.kindSymbol)
                    .font(.caption.weight(.medium))
                    .foregroundStyle(.secondary)
                Spacer()
                Text("from \(clip.source) · \(clip.createdAt, style: .relative)")
                    .font(.caption)
                    .foregroundStyle(.tertiary)
                    .lineLimit(1)
            }
            if clip.isText, let content = clip.content {
                Text(content)
                    .font(.callout)
                    .lineLimit(3)
                    .frame(maxWidth: .infinity, alignment: .leading)
                HStack {
                    Spacer()
                    Button("Copy") { viewModel.copyCurrentClip() }
                        .controlSize(.small)
                }
            }
        }
        .padding(10)
        .background(.quaternary.opacity(0.4), in: RoundedRectangle(cornerRadius: 10))
    }
}

private struct DeviceStatusTile: View {
    let device: Device

    private var avatarColor: Color {
        Color(hue: device.avatarHue, saturation: 0.55, brightness: 0.85)
    }

    var body: some View {
        VStack(spacing: 6) {
            Image(systemName: device.platformSymbol)
                .font(.system(size: 19, weight: .medium))
                .foregroundStyle(.white)
                .frame(width: 44, height: 44)
                .background(avatarColor.gradient, in: Circle())
                .overlay(alignment: .bottomTrailing) {
                    Circle()
                        .fill(device.online ? Color.green : Color.gray)
                        .frame(width: 10, height: 10)
                        .overlay(Circle().stroke(.background, lineWidth: 2))
                }
            Text(device.name)
                .font(.caption.weight(.medium))
                .lineLimit(1)
        }
        .frame(maxWidth: .infinity)
        .padding(.vertical, 10)
        .background(Color.primary.opacity(0.04), in: RoundedRectangle(cornerRadius: 12))
        .help(device.online ? "Online" : "Last seen \(device.lastSeen.formatted(.relative(presentation: .named)))")
        .accessibilityElement(children: .combine)
        .accessibilityLabel("\(device.name), \(device.online ? "online" : "offline")")
    }
}
