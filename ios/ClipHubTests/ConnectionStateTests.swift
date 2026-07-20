import XCTest
@testable import ClipHubKit

/// Phase 1 of the interface spec: only four human connection states may
/// ever reach a label — synced / connecting / offline / Tailscale off —
/// and raw error text is banned from UI copy.
final class ConnectionStateTests: XCTestCase {

    // MARK: - Human labels

    func testLabelsAreHumanStatesOnly() {
        XCTAssertEqual(ConnectionState.connected.label, "Synced")
        XCTAssertEqual(ConnectionState.connecting.label, "Connecting…")
        XCTAssertEqual(ConnectionState.disconnected(reason: "boom").label, "Offline — reconnecting…")
        XCTAssertEqual(ConnectionState.noVPN.label, "Tailscale is off")
    }

    /// The regression this prevents: the raw disconnect reason must never
    /// leak into the user-facing label.
    func testDisconnectedLabelOmitsRawReason() {
        let state = ConnectionState.disconnected(reason: "NSURLErrorDomain error -1009")
        XCTAssertFalse(state.label.contains("-1009"))
        XCTAssertFalse(state.label.contains("NSURLErrorDomain"))
        XCTAssertFalse(state.label.contains("Disconnected"))
    }

    func testIsConnected() {
        XCTAssertTrue(ConnectionState.connected.isConnected)
        XCTAssertFalse(ConnectionState.connecting.isConnected)
        XCTAssertFalse(ConnectionState.noVPN.isConnected)
    }

    // MARK: - Tailnet address heuristic

    func testTailnetDetectionForMagicDNS() {
        XCTAssertTrue(ConnectionState.looksLikeTailnetAddress(URL(string: "http://macbook.tail1234.ts.net:9437")!))
    }

    func testTailnetDetectionForCGNATRange() {
        XCTAssertTrue(ConnectionState.looksLikeTailnetAddress(URL(string: "http://100.64.1.2:9437")!))
        XCTAssertTrue(ConnectionState.looksLikeTailnetAddress(URL(string: "http://100.127.255.254")!))
    }

    func testTailnetDetectionRejectsNonTailnet() {
        // 100.x outside 100.64/10 is ordinary public space.
        XCTAssertFalse(ConnectionState.looksLikeTailnetAddress(URL(string: "http://100.200.1.1")!))
        XCTAssertFalse(ConnectionState.looksLikeTailnetAddress(URL(string: "http://192.168.1.10:9437")!))
        XCTAssertFalse(ConnectionState.looksLikeTailnetAddress(URL(string: "https://example.com")!))
    }

    // MARK: - Error → state mapping

    func testConnectivityFailureOnTailnetIsTailscaleOff() {
        let hub = URL(string: "http://100.64.1.2:9437")!
        let error = URLError(.cannotConnectToHost)
        XCTAssertEqual(ConnectionState.from(error: error, hubURL: hub), .noVPN)
        XCTAssertEqual(ConnectionState.from(error: URLError(.notConnectedToInternet), hubURL: hub), .noVPN)
        XCTAssertEqual(ConnectionState.from(error: URLError(.dnsLookupFailed), hubURL: hub), .noVPN)
    }

    func testServerFailureOnTailnetStaysOffline() {
        // A 500-class failure means Tailscale is fine; the hub is down.
        let hub = URL(string: "http://100.64.1.2:9437")!
        let error = ClipHubError.httpError(statusCode: 500, body: "boom")
        let state = ConnectionState.from(error: error, hubURL: hub)
        XCTAssertEqual(state, .disconnected(reason: error.localizedDescription))
    }

    func testConnectivityFailureOffTailnetStaysOffline() {
        let hub = URL(string: "http://192.168.1.10:9437")!
        let state = ConnectionState.from(error: URLError(.cannotConnectToHost), hubURL: hub)
        if case .disconnected = state {} else {
            XCTFail("expected .disconnected, got \(state)")
        }
    }
}
