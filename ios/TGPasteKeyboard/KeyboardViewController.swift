import UIKit
import SwiftUI
import TGClipboardKit

class KeyboardViewController: UIInputViewController {
    private var hostingController: UIHostingController<TGPasteView>?

    override func viewDidLoad() {
        super.viewDidLoad()

        let vm = KeyboardViewModel(proxy: textDocumentProxy)
        let tailPasteView = TGPasteView(
            viewModel: vm,
            showsGlobe: needsInputModeSwitchKey,
            onGlobe: { [weak self] in self?.advanceToNextInputMode() }
        )
        let host = UIHostingController(rootView: tailPasteView)
        host.view.translatesAutoresizingMaskIntoConstraints = false
        host.view.backgroundColor = .clear

        addChild(host)
        self.view.addSubview(host.view)
        host.didMove(toParent: self)

        NSLayoutConstraint.activate([
            host.view.leadingAnchor.constraint(equalTo: self.view.leadingAnchor),
            host.view.trailingAnchor.constraint(equalTo: self.view.trailingAnchor),
            host.view.topAnchor.constraint(equalTo: self.view.topAnchor),
            host.view.bottomAnchor.constraint(equalTo: self.view.bottomAnchor),
        ])

        // Designed height for the strip layout, deliberately below required
        // priority so the system can override it (iPad floating keyboard,
        // dictation bar) instead of trapping us in a broken frame.
        let height = self.view.heightAnchor.constraint(equalToConstant: 136)
        height.priority = .defaultHigh
        height.isActive = true

        self.hostingController = host
    }

    override func viewWillAppear(_ animated: Bool) {
        super.viewWillAppear(animated)
        hostingController?.rootView.showsGlobe = needsInputModeSwitchKey
        hostingController?.rootView.viewModel.refresh()
    }
}
