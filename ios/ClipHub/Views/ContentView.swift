import SwiftUI
import ClipHubKit

struct ContentView: View {
    @Environment(AppViewModel.self) private var viewModel
    @Environment(\.accessibilityReduceMotion) private var reduceMotion

    var body: some View {
        @Bindable var vm = viewModel

        Group {
            if vm.showOnboarding {
                // First run: full-screen, no way to skip configuration.
                OnboardingView(allowsCancel: false)
                    .transition(.opacity)
            } else {
                TabView(selection: $vm.selectedTab) {
                    CurrentClipView()
                        .tabItem { Label("Clipboard", systemImage: "doc.on.clipboard") }
                        .tag(AppTab.clipboard)

                    DevicesView()
                        .tabItem { Label("Devices", systemImage: "laptopcomputer.and.iphone") }
                        .tag(AppTab.devices)

                    SettingsView()
                        .tabItem { Label("Settings", systemImage: "gear") }
                        .tag(AppTab.settings)
                }
                .transition(.opacity)
                // Reconfiguration is a sheet: the user is never trapped and
                // can cancel back into the working app.
                .sheet(isPresented: $vm.isReconfiguring) {
                    OnboardingView(allowsCancel: true)
                }
            }
        }
        .animation(reduceMotion ? .none : .clipHub, value: vm.showOnboarding)
    }
}
