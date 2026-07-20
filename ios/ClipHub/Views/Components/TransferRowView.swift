import SwiftUI
import Foundation
import ClipHubKit

/// Incoming transfer row: shows what is actually being offered (names,
/// count, total size, sender, optional note) and gives live feedback while
/// receiving instead of just disabling the Accept button.
struct TransferRowView: View {
    let transfer: Transfer
    let fromName: String
    let isReceiving: Bool
    let accept: () -> Void
    let decline: () -> Void

    private var totalSize: Int64 {
        transfer.files.reduce(Int64(0)) { $0 + $1.size }
    }

    private var summary: String {
        let count = transfer.files.count
        let noun = count == 1 ? "file" : "files"
        let size = ByteCountFormatter.string(fromByteCount: totalSize, countStyle: .file)
        return "\(count) \(noun) · \(size) from \(fromName)"
    }

    var body: some View {
        VStack(alignment: .leading, spacing: 8) {
            Text(transfer.files.map(\.name).joined(separator: ", "))
                .font(.body)
                .lineLimit(2)

            Text(summary)
                .font(.footnote)
                .foregroundStyle(.secondary)

            if let note = transfer.note, !note.isEmpty {
                Text(note)
                    .font(.footnote)
                    .foregroundStyle(.secondary)
            }

            if isReceiving {
                HStack(spacing: 8) {
                    ProgressView()
                    Text("Receiving…")
                        .font(.footnote)
                        .foregroundStyle(.secondary)
                }
                .accessibilityElement(children: .combine)
                .accessibilityLabel("Receiving files")
            } else {
                HStack {
                    Button("Accept", action: accept)
                        .buttonStyle(.borderedProminent)
                    Button("Decline", role: .destructive, action: decline)
                        .buttonStyle(.bordered)
                }
            }
        }
        .padding(.vertical, 4)
    }
}
