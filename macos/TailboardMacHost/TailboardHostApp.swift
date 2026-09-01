import AppKit

/// A one-shot host for Tailboard's signed background login item. The host has
/// no menu, window, Dock icon, or login registration of its own: launching it
/// registers or updates Tailboard Engine and then exits.
@main
@MainActor
final class TailboardHostApp: NSObject, NSApplicationDelegate {
    static func main() {
        let application = NSApplication.shared
        let delegate = TailboardHostApp()
        application.delegate = delegate
        application.setActivationPolicy(.prohibited)
        withExtendedLifetime(delegate) {
            application.run()
        }
    }

    func applicationDidFinishLaunching(_ notification: Notification) {
        Task {
            do {
                try await EngineServiceManager.shared.activate()
            } catch {
                UserDefaults.standard.set(
                    error.localizedDescription,
                    forKey: EngineServiceManager.lastErrorKey
                )
                present(error)
            }
            NSApp.terminate(nil)
        }
    }

    private func present(_ error: Error) {
        let alert = NSAlert()
        alert.alertStyle = .warning
        alert.messageText = "Tailboard Engine could not start"
        alert.informativeText = error.localizedDescription
        if case EngineServiceError.approvalRequired = error {
            alert.addButton(withTitle: "Open Login Items")
            alert.addButton(withTitle: "Close")
            if alert.runModal() == .alertFirstButtonReturn {
                EngineServiceManager.shared.openLoginItemsSettings()
            }
        } else {
            alert.addButton(withTitle: "Close")
            alert.runModal()
        }
    }
}
