import SwiftUI
import UniformTypeIdentifiers

@main
struct TGClipboardMenuBarApp: App {
    @State private var viewModel = MenuBarViewModel()

    var body: some Scene {
        MenuBarExtra {
            PopoverView()
                .environment(viewModel)
        } label: {
            // Dropping files on the menu bar icon stages them and opens the
            // popover in "pick a device" mode — the fastest send path.
            Image("TailboardMenuBar")
                // Default SF Symbol rendering is oversized next to system
                // status items; match their ~13pt optical size.
                .resizable()
                .renderingMode(.template)
                .frame(width: 16, height: 16)
                .onDrop(of: [.fileURL], isTargeted: nil) { providers in
                    stageDrop(providers)
                }
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

    private func stageDrop(_ providers: [NSItemProvider]) -> Bool {
        let group = DispatchGroup()
        var urls: [URL] = []
        let lock = NSLock()
        for provider in providers where provider.hasItemConformingToTypeIdentifier(UTType.fileURL.identifier) {
            group.enter()
            _ = provider.loadObject(ofClass: URL.self) { url, _ in
                if let url {
                    lock.lock(); urls.append(url); lock.unlock()
                }
                group.leave()
            }
        }
        group.notify(queue: .main) {
            guard !urls.isEmpty else { return }
            viewModel.stagedFileURLs = urls
        }
        return !providers.isEmpty
    }
}
