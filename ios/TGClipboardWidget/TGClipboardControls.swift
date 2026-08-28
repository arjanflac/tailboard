import AppIntents
import SwiftUI
import WidgetKit
import TGClipboardKit

// Keep the control actions in the widget-extension target itself. A plain
// AppIntent invoked by a WidgetKit control executes in that extension process;
// framework-only intent metadata can render a control that never dispatches.
@available(iOS 18.0, *)
struct ControlPushClipboardIntent: AppIntent {
    static var title: LocalizedStringResource = "Send Clipboard"
    static var description = IntentDescription("Sends this iPhone's clipboard through Tailboard.")
    static var openAppWhenRun: Bool { true }

    func perform() async throws -> some IntentResult {
        let store = AppGroupStore()
        store.requestControlAction("send")
        store.recordControlDiagnostic(
            action: "Send",
            status: "Opening Tailboard",
            detail: "Clipboard access completes in the foreground app"
        )
        return .result()
    }
}

@available(iOS 18.0, *)
struct ControlReceiveClipboardIntent: AppIntent {
    static var title: LocalizedStringResource = "Receive Clipboard"
    static var description = IntentDescription("Copies the latest synced clipboard onto this iPhone.")
    static var openAppWhenRun: Bool { true }

    func perform() async throws -> some IntentResult {
        let store = AppGroupStore()
        store.requestControlAction("receive")
        store.recordControlDiagnostic(
            action: "Receive",
            status: "Opening Tailboard",
            detail: "Clipboard access completes in the foreground app"
        )
        return .result()
    }
}

@available(iOS 18.0, *)
struct SendClipboardControl: ControlWidget {
    static let kind = "com.arjanflac.tgclipboard.control.send"

    var body: some ControlWidgetConfiguration {
        StaticControlConfiguration(kind: Self.kind) {
            ControlWidgetButton(action: ControlPushClipboardIntent()) {
                Label("Send Clipboard", systemImage: "doc.on.clipboard.fill")
                    .controlWidgetStatus("Sent")
                    .controlWidgetActionHint("Send clipboard")
            }
        }
        .displayName("Send Clipboard")
        .description("Send the iPhone clipboard through Tailboard.")
    }
}

@available(iOS 18.0, *)
struct ReceiveClipboardControl: ControlWidget {
    static let kind = "com.arjanflac.tgclipboard.control.receive"

    var body: some ControlWidgetConfiguration {
        StaticControlConfiguration(kind: Self.kind) {
            ControlWidgetButton(action: ControlReceiveClipboardIntent()) {
                Label("Receive Clipboard", systemImage: "arrow.down.doc.fill")
                    .controlWidgetStatus("Copied")
                    .controlWidgetActionHint("Copy the latest synced clipboard")
            }
        }
        .displayName("Receive Clipboard")
        .description("Copy the latest synced clipboard onto this iPhone.")
    }
}
