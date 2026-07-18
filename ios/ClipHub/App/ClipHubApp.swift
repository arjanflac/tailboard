import SwiftUI

@main
struct ClipHubApp: App {
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
                    if url.host == "copy-current" {
                        viewModel.copyCurrentToPasteboard()
                    }
                }
        }
    }
}
