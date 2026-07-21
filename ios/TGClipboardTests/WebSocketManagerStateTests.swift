import XCTest
@testable import TGClipboardKit

final class WebSocketManagerStateTests: XCTestCase {

    /// The app view model binds its visible connection status to this
    /// callback; regressions here silently freeze the Settings status row.
    func testConnectionStateChangeCallbackFiresOnEveryTransition() {
        let manager = WebSocketManager()
        var received: [ConnectionState] = []
        manager.onConnectionStateChange = { received.append($0) }

        manager.connectionState = .connecting
        manager.connectionState = .connected
        manager.connectionState = .disconnected(reason: "boom")

        XCTAssertEqual(received, [
            .connecting,
            .connected,
            .disconnected(reason: "boom"),
        ])
    }

    func testStopReportsDisconnectedThroughCallback() {
        let manager = WebSocketManager()
        var received: ConnectionState?
        manager.onConnectionStateChange = { received = $0 }

        manager.stop()

        XCTAssertEqual(received, .disconnected(reason: "Stopped"))
    }
}
