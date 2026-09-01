import SwiftUI
import WidgetKit
import TGClipboardKit

@main
struct TGClipboardApp: App {
    @State private var viewModel = AppViewModel()
    @Environment(\.scenePhase) private var scenePhase

    var body: some Scene {
        WindowGroup {
            ContentView()
                .environment(viewModel)
                .onChange(of: scenePhase) { _, phase in
                    viewModel.setSceneActive(phase == .active)
                    if phase == .active {
                        Task { await viewModel.handlePendingControlAction() }
                    }
                }
                .task {
                    await viewModel.finishPrivacyWipe()
                    await viewModel.endLegacyLiveActivities()
                    viewModel.setSceneActive(scenePhase == .active)
                    await viewModel.handlePendingControlAction()
                    if #available(iOS 18.0, *) {
                        // Refresh existing Control Center placements after an
                        // app update so they pick up the extension-local
                        // intent metadata without needing to be deleted.
                        ControlCenter.shared.reloadAllControls()
                        if let controls = try? await ControlCenter.shared.currentControls() {
                            AppGroupStore().recordConfiguredControls(controls.map(\.kind))
                        }
                    }
                }
                .onOpenURL { url in
                    viewModel.handleDeepLink(url)
                }
        }
    }
}
