import SwiftUI

@main
struct TGClipboardMenuBarApp: App {
    @State private var viewModel = MenuBarViewModel()

    var body: some Scene {
        MenuBarExtra {
            PopoverView()
                .environment(viewModel)
        } label: {
            Image("TailboardMenuBar")
                .resizable()
                .renderingMode(.template)
                .frame(width: 16, height: 16)
                .task {
                    do {
                        try await EngineServiceManager.shared.activate()
                    } catch {
                        UserDefaults.standard.set(
                            error.localizedDescription,
                            forKey: EngineServiceManager.lastErrorKey
                        )
                        viewModel.errorMessage = error.localizedDescription
                    }
                    viewModel.start()
                }
        }
        .menuBarExtraStyle(.window)
    }
}
