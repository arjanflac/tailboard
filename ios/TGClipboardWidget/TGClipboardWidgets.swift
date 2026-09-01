import SwiftUI
import WidgetKit
import TGClipboardKit

private struct ClipEntry: TimelineEntry {
    let date: Date
    let preview: String?
    let source: String
}

private struct ClipProvider: TimelineProvider {
    func placeholder(in context: Context) -> ClipEntry {
        ClipEntry(date: .now, preview: "Clipboard preview", source: "MacBook")
    }

    func getSnapshot(in context: Context, completion: @escaping (ClipEntry) -> Void) {
        completion(entry())
    }

    func getTimeline(in context: Context, completion: @escaping (Timeline<ClipEntry>) -> Void) {
        completion(Timeline(entries: [entry()], policy: .after(.now.addingTimeInterval(15 * 60))))
    }

    private func entry() -> ClipEntry {
        let clip = AppGroupStore().cachedCurrentClip
        return ClipEntry(
            date: clip?.createdAt ?? .now,
            preview: clip?.preview,
            source: clip?.source ?? "Tailboard"
        )
    }
}

/// Applies the iOS 17 container background only to home-screen families;
/// accessory (lock screen) widgets render in the system's masked, monochrome
/// context and must not bring their own container.
private struct WidgetContainer: ViewModifier {
    let family: WidgetFamily

    @ViewBuilder
    func body(content: Content) -> some View {
        switch family {
        case .accessoryRectangular, .accessoryCircular, .accessoryInline:
            content
        default:
            content.containerBackground(.fill.tertiary, for: .widget)
        }
    }
}

private struct ClipWidgetView: View {
    let entry: ClipEntry

    @Environment(\.widgetFamily) private var family

    var body: some View {
        content
            .modifier(WidgetContainer(family: family))
            .widgetURL(URL(string: "tgclipboard://copy-current"))
    }

    @ViewBuilder
    private var content: some View {
        switch family {
        case .accessoryRectangular:
            accessoryBody
        case .systemMedium:
            mediumBody
        default:
            smallBody
        }
    }

    // MARK: - Accessory (lock screen): no container, compact, monochrome

    private var accessoryBody: some View {
        VStack(alignment: .leading, spacing: 2) {
            Label("Tailboard", systemImage: "doc.on.clipboard")
                .font(.caption2.weight(.semibold))
            if let preview = entry.preview {
                Text(preview)
                    .font(.caption)
                    .lineLimit(2)
                    .minimumScaleFactor(0.8)
                    .privacySensitive()
            } else {
                Text("Nothing copied yet")
                    .font(.caption2)
                    .foregroundStyle(.secondary)
            }
        }
        .frame(maxWidth: .infinity, alignment: .leading)
    }

    // MARK: - systemSmall

    private var smallBody: some View {
        VStack(alignment: .leading, spacing: 6) {
            header
            if let preview = entry.preview {
                Text(preview)
                    .font(.callout)
                    .lineLimit(3)
                    .minimumScaleFactor(0.8)
                    .privacySensitive()
                Spacer(minLength: 0)
                Text(entry.date, style: .relative)
                    .font(.caption2)
                    .foregroundStyle(.secondary)
            } else {
                emptyState
            }
        }
        .frame(maxWidth: .infinity, alignment: .leading)
    }

    // MARK: - systemMedium

    private var mediumBody: some View {
        VStack(alignment: .leading, spacing: 6) {
            header
            if let preview = entry.preview {
                Text(preview)
                    .font(.body)
                    .lineLimit(3)
                    .minimumScaleFactor(0.8)
                    .privacySensitive()
                Spacer(minLength: 0)
                HStack(spacing: 4) {
                    Text("from \(entry.source)")
                    Text("·")
                    Text(entry.date, style: .relative)
                }
                .font(.caption2)
                .foregroundStyle(.secondary)
            } else {
                emptyState
            }
        }
        .frame(maxWidth: .infinity, alignment: .leading)
    }

    // MARK: - Pieces

    private var header: some View {
        HStack(spacing: 4) {
            Image(systemName: "doc.on.clipboard")
                .foregroundStyle(Color.accentColor)
            Text("Tailboard")
        }
        .font(.caption.weight(.semibold))
        .accessibilityHidden(true)
    }

    private var emptyState: some View {
        VStack(alignment: .leading, spacing: 4) {
            Spacer(minLength: 0)
            Image(systemName: "doc.on.clipboard")
                .font(.title3)
                .foregroundStyle(Color.accentColor)
            Text("Nothing copied yet")
                .font(.caption)
                .foregroundStyle(.secondary)
            Text("Copy on any device")
                .font(.caption2)
                .foregroundStyle(.tertiary)
            Spacer(minLength: 0)
        }
        .frame(maxWidth: .infinity)
    }
}

private struct CurrentClipWidget: Widget {
    let kind = "CurrentClipWidget"

    var body: some WidgetConfiguration {
        StaticConfiguration(kind: kind, provider: ClipProvider()) { entry in
            ClipWidgetView(entry: entry)
        }
        .configurationDisplayName("Current Clip")
        .description("Shows the latest cached Tailboard item. Tap to copy it.")
        .supportedFamilies([.systemSmall, .systemMedium, .accessoryRectangular])
    }
}

@main
struct TGClipboardWidgetBundle: WidgetBundle {
    var body: some Widget {
        CurrentClipWidget()
        if #available(iOS 18.0, *) {
            SendClipboardControl()
            ReceiveClipboardControl()
        }
    }
}
