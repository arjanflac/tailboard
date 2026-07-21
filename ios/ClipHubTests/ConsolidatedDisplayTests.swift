import XCTest
@testable import ClipHubKit

/// Tests for the consolidation follow-up (ios/UX_INTEGRATION_REVIEW.md P1-2):
/// the main app now renders ClipItem.kindSymbol and Device.platformLabel, so
/// these mappings are load-bearing across every surface.
final class ConsolidatedDisplayTests: XCTestCase {

    private func makeClip(mimeType: String, content: String? = nil) throws -> ClipItem {
        var json = """
        {
            "seq": 1,
            "mime_type": "\(mimeType)",
            "hash": "abc",
            "source": "mac",
            "created_at": "2026-03-16T12:00:00Z",
            "expires_at": "2026-03-17T12:00:00Z"
        """
        if let content {
            json += ",\n    \"content\": \"\(content)\""
        }
        json += "\n}"
        let decoder = JSONDecoder()
        decoder.dateDecodingStrategy = .clipHubISO8601
        return try decoder.decode(ClipItem.self, from: Data(json.utf8))
    }

    private func makeDevice(platform: String) throws -> Device {
        let json = """
        {
            "device_id": "d1",
            "name": "Test",
            "platform": "\(platform)",
            "capabilities": [],
            "online": true,
            "last_seen": "2026-03-16T12:00:00Z"
        }
        """
        let decoder = JSONDecoder()
        decoder.dateDecodingStrategy = .clipHubISO8601
        return try decoder.decode(Device.self, from: Data(json.utf8))
    }

    // MARK: - ClipItem.kindSymbol

    func testKindSymbolForPlainText() throws {
        XCTAssertEqual(try makeClip(mimeType: "text/plain", content: "hello").kindSymbol, "doc.text")
    }

    func testKindSymbolForLink() throws {
        XCTAssertEqual(try makeClip(mimeType: "text/plain", content: "https://example.com").kindSymbol, "link")
    }

    func testKindSymbolForImage() throws {
        XCTAssertEqual(try makeClip(mimeType: "image/png").kindSymbol, "photo")
    }

    func testKindSymbolForBinaryFile() throws {
        XCTAssertEqual(try makeClip(mimeType: "application/pdf").kindSymbol, "doc")
    }

    // MARK: - Device.platformLabel

    func testPlatformLabelMapping() throws {
        XCTAssertEqual(try makeDevice(platform: "ios").platformLabel, "iOS")
        XCTAssertEqual(try makeDevice(platform: "darwin").platformLabel, "Mac")
        XCTAssertEqual(try makeDevice(platform: "windows").platformLabel, "Windows")
        XCTAssertEqual(try makeDevice(platform: "linux").platformLabel, "Linux")
    }

    func testPlatformLabelFallbackCapitalizes() throws {
        XCTAssertEqual(try makeDevice(platform: "plan9").platformLabel, "Plan9")
    }

    /// The regression this prevents: raw platform identifiers like "darwin"
    /// must never reach user-facing text again.
    func testPlatformLabelNeverEqualsRawIdentifier() throws {
        for raw in ["ios", "darwin", "windows", "linux"] {
            XCTAssertNotEqual(try makeDevice(platform: raw).platformLabel, raw)
        }
    }
}
