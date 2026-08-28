import SwiftUI
import UniformTypeIdentifiers
import TGClipboardKit

/// The menu bar popover (spec §4.2): current clip card, device drop grid,
/// activity, footer. Drag a file onto a device and it arrives — that's it.
struct PopoverView: View {
    @Environment(MenuBarViewModel.self) private var viewModel

    var body: some View {
        VStack(alignment: .leading, spacing: 14) {
            if viewModel.status == .engineOff {
                engineOffCard
            } else if viewModel.status == .noTailscale {
                noTailscaleCard
            } else {
                if !viewModel.stagedFileURLs.isEmpty {
                    stagedBanner
                }
                if let clip = viewModel.state?.clip {
                    ClipCard(clip: clip, size: viewModel.state?.clipSize ?? 0)
                }
                devicesSection
                if !viewModel.incomingOffers.isEmpty || !viewModel.activeTransfers.isEmpty {
                    activitySection
                }
                if let message = viewModel.errorMessage {
                    errorRow(message)
                }
            }
            footer
        }
        .padding(14)
        .frame(width: 320)
        .task { await viewModel.refresh() }
    }

    // MARK: - Engine off

    private var engineOffCard: some View {
        VStack(spacing: 10) {
            Image(systemName: "powerplug")
                .font(.system(size: 28))
                .foregroundStyle(.secondary)
            Text("Tailboard Engine isn't running")
                .font(.headline)
            Text("Start it to sync your clipboard and send files.")
                .font(.caption)
                .foregroundStyle(.secondary)
                .multilineTextAlignment(.center)
            Button("Start") { viewModel.startEngine() }
                .buttonStyle(.borderedProminent)
        }
        .frame(maxWidth: .infinity)
        .padding(.vertical, 20)
    }

    // MARK: - Tailscale off

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
                .multilineTextAlignment(.center)
            Button("Open Tailscale") { viewModel.openTailscale() }
                .buttonStyle(.borderedProminent)
        }
        .frame(maxWidth: .infinity)
        .padding(.vertical, 20)
    }

    // MARK: - Staged drop

    private var stagedBanner: some View {
        HStack(spacing: 8) {
            Image(systemName: "arrow.down.doc.fill")
                .foregroundStyle(Color.accentColor)
            Text(viewModel.stagedFileURLs.count == 1
                 ? "Pick a device for “\(viewModel.stagedFileURLs[0].lastPathComponent)”"
                 : "Pick a device for \(viewModel.stagedFileURLs.count) files")
                .font(.callout.weight(.medium))
                .lineLimit(1)
            Spacer(minLength: 0)
            Button {
                viewModel.stagedFileURLs = []
            } label: {
                Image(systemName: "xmark.circle.fill")
                    .foregroundStyle(.secondary)
            }
            .buttonStyle(.plain)
            .accessibilityLabel("Cancel send")
        }
        .padding(10)
        .background(Color.accentColor.opacity(0.12), in: RoundedRectangle(cornerRadius: 10, style: .continuous))
    }

    // MARK: - Devices

    private var devicesSection: some View {
        VStack(alignment: .leading, spacing: 8) {
            Text(viewModel.stagedFileURLs.isEmpty ? "Drop files on a device" : "Send to…")
                .font(.caption.weight(.semibold))
                .foregroundStyle(.secondary)
                .textCase(.uppercase)

            if viewModel.otherDevices.isEmpty {
                VStack(spacing: 6) {
                    Text("No devices yet")
                        .font(.callout.weight(.medium))
                    Text("Install Tailboard on your phone — it finds this Mac automatically.")
                        .font(.caption)
                        .foregroundStyle(.secondary)
                        .multilineTextAlignment(.center)
                }
                .frame(maxWidth: .infinity)
                .padding(.vertical, 16)
            } else {
                LazyVGrid(columns: [GridItem(.adaptive(minimum: 90), spacing: 8)], spacing: 8) {
                    ForEach(viewModel.otherDevices) { device in
                        DeviceDropTile(device: device)
                    }
                }
            }
        }
    }

    // MARK: - Activity

    private var activitySection: some View {
        VStack(alignment: .leading, spacing: 8) {
            Text("Activity")
                .font(.caption.weight(.semibold))
                .foregroundStyle(.secondary)
                .textCase(.uppercase)

            ForEach(viewModel.incomingOffers) { offer in
                OfferRow(offer: offer)
            }
            ForEach(viewModel.activeTransfers) { transfer in
                TransferProgressRow(transfer: transfer)
            }
        }
    }

    private func errorRow(_ message: String) -> some View {
        HStack(spacing: 6) {
            Image(systemName: "exclamationmark.triangle.fill")
                .foregroundStyle(.orange)
            Text(message)
                .font(.caption)
                .lineLimit(2)
            Spacer(minLength: 0)
            Button {
                viewModel.errorMessage = nil
            } label: {
                Image(systemName: "xmark")
                    .font(.caption2)
            }
            .buttonStyle(.plain)
        }
        .padding(8)
        .background(.quaternary.opacity(0.5), in: RoundedRectangle(cornerRadius: 8))
    }

    // MARK: - Footer

    private var footer: some View {
        HStack {
            Text(viewModel.statusLine)
                .font(.caption)
                .foregroundStyle(.secondary)
            Spacer()
            if viewModel.status != .engineOff {
                Button {
                    viewModel.togglePaused()
                } label: {
                    Image(systemName: viewModel.status == .paused ? "play.fill" : "pause.fill")
                }
                .buttonStyle(.plain)
                .foregroundStyle(.secondary)
                .help(viewModel.status == .paused ? "Resume clipboard sync" : "Pause clipboard sync")
                Button {
                    viewModel.revealDownloads()
                } label: {
                    Image(systemName: "folder")
                }
                .buttonStyle(.plain)
                .foregroundStyle(.secondary)
                .help("Open Tailboard Downloads")
            }
            Button {
                NSApp.terminate(nil)
            } label: {
                Image(systemName: "power")
            }
            .buttonStyle(.plain)
            .foregroundStyle(.secondary)
            .help("Quit Tailboard")
        }
    }
}

// MARK: - Clip card

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
        .background(.quaternary.opacity(0.4), in: RoundedRectangle(cornerRadius: 10, style: .continuous))
    }
}

// MARK: - Device tile (drop target)

struct DeviceDropTile: View {
    @Environment(MenuBarViewModel.self) private var viewModel
    let device: Device

    @State private var isDropTargeted = false

    private var isSending: Bool { viewModel.sendingDeviceIDs.contains(device.deviceID) }
    private var justSent: Bool { viewModel.justSentDeviceID == device.deviceID }
    private var avatarColor: Color {
        Color(hue: device.avatarHue, saturation: 0.55, brightness: 0.85)
    }

    var body: some View {
        Button(action: primaryAction) {
            VStack(spacing: 6) {
                ZStack {
                    Circle()
                        .fill(avatarColor.gradient)
                        .frame(width: 44, height: 44)
                    if isSending {
                        ProgressView()
                            .controlSize(.small)
                            .tint(.white)
                    } else if justSent {
                        Image(systemName: "checkmark")
                            .font(.system(size: 18, weight: .bold))
                            .foregroundStyle(.white)
                    } else {
                        Image(systemName: device.platformSymbol)
                            .font(.system(size: 19, weight: .medium))
                            .foregroundStyle(.white)
                    }
                }
                .overlay(alignment: .bottomTrailing) {
                    Circle()
                        .fill(device.online ? Color.green : Color.gray)
                        .frame(width: 10, height: 10)
                        .overlay(Circle().stroke(.background, lineWidth: 2))
                }
                Text(device.name)
                    .font(.caption.weight(.medium))
                    .lineLimit(1)
                    .truncationMode(.tail)
            }
            .frame(maxWidth: .infinity)
            .padding(.vertical, 10)
            .padding(.horizontal, 4)
            .background(
                isDropTargeted ? avatarColor.opacity(0.25) : Color.primary.opacity(0.04),
                in: RoundedRectangle(cornerRadius: 12, style: .continuous)
            )
            .overlay {
                if isDropTargeted {
                    RoundedRectangle(cornerRadius: 12, style: .continuous)
                        .stroke(avatarColor, style: StrokeStyle(lineWidth: 2, dash: [5]))
                }
            }
            .animation(.easeOut(duration: 0.15), value: isDropTargeted)
        }
        .buttonStyle(.plain)
        .disabled(isSending)
        .onDrop(of: [.fileURL], isTargeted: $isDropTargeted) { providers in
            handleDrop(providers)
        }
        .accessibilityLabel("\(device.name), \(device.online ? "online" : "offline")")
        .accessibilityHint("Click to send files, or drop files here")
        .help(device.online
              ? "Drop files to send to \(device.name)"
              : "Last seen \(device.lastSeen.formatted(.relative(presentation: .named)))")
    }

    private func primaryAction() {
        if !viewModel.stagedFileURLs.isEmpty {
            viewModel.sendStaged(to: device)
        } else {
            viewModel.pickAndSendFiles(to: device)
        }
    }

    private func handleDrop(_ providers: [NSItemProvider]) -> Bool {
        let group = DispatchGroup()
        var urls: [URL] = []
        let lock = NSLock()
        for provider in providers where provider.hasItemConformingToTypeIdentifier(UTType.fileURL.identifier) {
            group.enter()
            _ = provider.loadObject(ofClass: URL.self) { url, _ in
                if let url {
                    lock.lock(); urls.append(url); lock.unlock()
                }
                group.leave()
            }
        }
        group.notify(queue: .main) {
            guard !urls.isEmpty else { return }
            viewModel.send(fileURLs: urls, to: device)
        }
        return !providers.isEmpty
    }
}

// MARK: - Activity rows

private struct OfferRow: View {
    @Environment(MenuBarViewModel.self) private var viewModel
    let offer: Transfer

    private var fromName: String {
        (viewModel.state?.devices ?? []).first { $0.deviceID == offer.fromDevice }?.name ?? "Unknown device"
    }

    var body: some View {
        HStack(spacing: 8) {
            Image(systemName: "tray.and.arrow.down")
                .foregroundStyle(Color.accentColor)
            VStack(alignment: .leading, spacing: 2) {
                Text("\(fromName) wants to send \(MenuBarViewModel.filesPhrase(offer))")
                    .font(.caption.weight(.medium))
                    .lineLimit(1)
                Text(offer.files.map(\.name).joined(separator: ", "))
                    .font(.caption2)
                    .foregroundStyle(.secondary)
                    .lineLimit(1)
            }
            Spacer(minLength: 4)
            Button("Accept") { viewModel.accept(offer) }
                .controlSize(.small)
                .buttonStyle(.borderedProminent)
            Button {
                viewModel.decline(offer)
            } label: {
                Image(systemName: "xmark")
            }
            .controlSize(.small)
            .help("Decline")
        }
        .padding(8)
        .background(.quaternary.opacity(0.4), in: RoundedRectangle(cornerRadius: 10))
    }
}

private struct TransferProgressRow: View {
    @Environment(MenuBarViewModel.self) private var viewModel
    let transfer: Transfer

    private var totalSize: Int64 { transfer.files.reduce(0) { $0 + $1.size } }
    private var uploaded: Int64 { transfer.files.reduce(0) { $0 + $1.uploaded } }
    private var complete: Bool { transfer.state == "complete" }
    private var peerName: String {
        let peer = transfer.fromDevice == viewModel.deviceID ? transfer.toDevice : transfer.fromDevice
        return (viewModel.state?.devices ?? []).first { $0.deviceID == peer }?.name ?? "Unknown device"
    }

    var body: some View {
        HStack(spacing: 8) {
            Image(systemName: complete ? "checkmark.circle.fill" : "arrow.up.arrow.down.circle")
                .foregroundStyle(complete ? .green : Color.accentColor)
            VStack(alignment: .leading, spacing: 3) {
                Text(complete
                     ? "Saved — \(transfer.files.map(\.name).joined(separator: ", "))"
                     : (transfer.fromDevice == viewModel.deviceID
                        ? "Sending to \(peerName)…"
                        : "Receiving from \(peerName)…"))
                    .font(.caption.weight(.medium))
                    .lineLimit(1)
                if !complete, totalSize > 0 {
                    ProgressView(value: Double(uploaded), total: Double(totalSize))
                        .controlSize(.small)
                }
            }
            Spacer(minLength: 4)
            if complete {
                Button("Show") { viewModel.revealDownloads() }
                    .controlSize(.small)
            }
        }
        .padding(8)
        .background(.quaternary.opacity(0.4), in: RoundedRectangle(cornerRadius: 10))
    }
}
