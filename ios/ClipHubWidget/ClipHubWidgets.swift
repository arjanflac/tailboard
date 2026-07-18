import ActivityKit
import SwiftUI
import WidgetKit
import ClipHubKit

private struct ClipEntry: TimelineEntry {
    let date: Date
    let preview: String
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
            preview: clip?.preview ?? "No current clip",
            source: clip?.source ?? "ClipHub"
        )
    }
}

private struct ClipWidgetView: View {
    let entry: ClipEntry

    var body: some View {
        VStack(alignment: .leading, spacing: 6) {
            Label("ClipHub", systemImage: "doc.on.clipboard")
                .font(.caption.weight(.semibold))
            Text(entry.preview)
                .font(.body)
                .lineLimit(3)
            Spacer(minLength: 0)
            Text("from \(entry.source) · \(entry.date, style: .relative)")
                .font(.caption2)
                .foregroundStyle(.secondary)
        }
        .containerBackground(.fill.tertiary, for: .widget)
        .widgetURL(URL(string: "cliphub://copy-current"))
    }
}

private struct CurrentClipWidget: Widget {
    let kind = "CurrentClipWidget"

    var body: some WidgetConfiguration {
        StaticConfiguration(kind: kind, provider: ClipProvider()) { entry in
            ClipWidgetView(entry: entry)
        }
        .configurationDisplayName("Current Clip")
        .description("Shows the latest cached ClipHub item. Tap to copy it.")
        .supportedFamilies([.systemSmall, .systemMedium, .accessoryRectangular])
    }
}

private struct ClipHubLiveActivity: Widget {
    var body: some WidgetConfiguration {
        ActivityConfiguration(for: ClipHubActivityAttributes.self) { context in
            VStack(alignment: .leading, spacing: 4) {
                Text(context.attributes.title)
                    .font(.caption.weight(.semibold))
                Text(context.state.preview)
                    .lineLimit(2)
                Text("from \(context.state.source)")
                    .font(.caption2)
                    .foregroundStyle(.secondary)
            }
            .padding()
            .activityBackgroundTint(.black.opacity(0.08))
            .widgetURL(URL(string: "cliphub://copy-current"))
        } dynamicIsland: { context in
            DynamicIsland {
                DynamicIslandExpandedRegion(.leading) {
                    Image(systemName: "doc.on.clipboard")
                }
                DynamicIslandExpandedRegion(.center) {
                    Text(context.state.preview).lineLimit(1)
                }
                DynamicIslandExpandedRegion(.bottom) {
                    Text("from \(context.state.source)").font(.caption)
                }
            } compactLeading: {
                Image(systemName: "doc.on.clipboard")
            } compactTrailing: {
                Text("Clip")
            } minimal: {
                Image(systemName: "doc.on.clipboard")
            }
            .widgetURL(URL(string: "cliphub://copy-current"))
        }
    }
}

@main
struct ClipHubWidgetBundle: WidgetBundle {
    var body: some Widget {
        CurrentClipWidget()
        ClipHubLiveActivity()
    }
}
