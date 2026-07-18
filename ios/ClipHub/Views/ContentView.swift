import SwiftUI
import ClipHubKit

struct ContentView: View {
    @Environment(AppViewModel.self) private var viewModel

    var body: some View {
        if viewModel.showOnboarding {
            OnboardingView()
        } else {
            TabView {
                DevicesView()
                    .tabItem { Label("Devices", systemImage: "laptopcomputer.and.iphone") }

                CurrentClipView()
                    .tabItem { Label("Current", systemImage: "doc.on.clipboard") }

                HistoryView()
                    .tabItem { Label("History", systemImage: "clock") }

                SettingsView()
                    .tabItem { Label("Settings", systemImage: "gear") }
            }
        }
    }
}
