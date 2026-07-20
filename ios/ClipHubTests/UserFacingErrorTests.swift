import XCTest
@testable import ClipHubKit

/// Spec principle 4: raw error.localizedDescription never reaches a label.
final class UserFacingErrorTests: XCTestCase {

    func testConnectivityFailuresMentionTailscaleNotJargon() {
        let message = UserFacingError.message(
            ClipHubError.hubUnreachable(underlying: URLError(.notConnectedToInternet))
        )
        XCTAssertTrue(message.contains("Tailscale"))
        XCTAssertFalse(message.contains("-1009"))
        XCTAssertFalse(message.contains("NSURLErrorDomain"))
    }

    func testHTTPServerErrorIsHuman() {
        let message = UserFacingError.message(
            ClipHubError.httpError(statusCode: 500, body: "goroutine panic")
        )
        XCTAssertFalse(message.contains("goroutine"))
        XCTAssertFalse(message.contains("HTTP"))
    }

    func testUnknownErrorFallsBackToGenericCopy() {
        struct Weird: Error {}
        XCTAssertEqual(UserFacingError.message(Weird()), "Something went wrong. Try again.")
    }
}
