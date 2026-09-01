import SwiftUI

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
                }
                .task {
                    await viewModel.finishPrivacyWipe()
                    viewModel.setSceneActive(scenePhase == .active)
                }
        }
    }
}
