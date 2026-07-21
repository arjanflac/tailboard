import SwiftUI
import UIKit
import TGClipboardKit

final class TGClipboardAppDelegate: NSObject, UIApplicationDelegate {
    func application(
        _ application: UIApplication,
        handleEventsForBackgroundURLSession identifier: String,
        completionHandler: @escaping () -> Void
    ) {
        guard identifier == BackgroundUploadCoordinator.sessionIdentifier else {
            completionHandler()
            return
        }
        BackgroundUploadCoordinator.shared.reconnect(
            completionHandler: completionHandler
        )
    }
}

@main
struct TGClipboardApp: App {
    @UIApplicationDelegateAdaptor(TGClipboardAppDelegate.self) private var appDelegate
    @State private var viewModel = AppViewModel()
    @Environment(\.scenePhase) private var scenePhase

    var body: some Scene {
        WindowGroup {
            ContentView()
                .environment(viewModel)
                .onChange(of: scenePhase) { _, phase in
                    viewModel.setSceneActive(phase == .active)
                }
                .task {
                    viewModel.setSceneActive(scenePhase == .active)
                }
                .onOpenURL { url in
                    viewModel.handleDeepLink(url)
                }
        }
    }
}
