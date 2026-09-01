import XCTest
@testable import TGClipboardKit

final class DisplayHelperTests: XCTestCase {
    private func makeClip(_ content: String) throws -> ClipItem {
        let json = """
        {
            "seq": 1,
            "content": "\(content)",
            "hash": "abc",
            "source": "mac",
            "created_at": "2026-03-16T12:00:00Z",
            "expires_at": "2026-03-17T12:00:00Z"
        }
        """
        let decoder = JSONDecoder()
        decoder.dateDecodingStrategy = .tgSpringISO8601
        return try decoder.decode(ClipItem.self, from: Data(json.utf8))
    }

    func testPlainTextSummary() throws {
        let clip = try makeClip("hello world")
        XCTAssertEqual(clip.displaySummary, "Text")
        XCTAssertEqual(clip.kindSymbol, "doc.text")
        XCTAssertFalse(clip.isLink)
    }

    func testLinkSummary() throws {
        let clip = try makeClip("https://example.com/path?q=1")
        XCTAssertEqual(clip.displaySummary, "Link")
        XCTAssertEqual(clip.kindSymbol, "link")
        XCTAssertTrue(clip.isLink)
    }

    func testLinkRequiresSchemeAndRejectsWhitespace() throws {
        XCTAssertFalse(try makeClip("example.com/path").isLink)
        XCTAssertFalse(try makeClip("see https://example.com").isLink)
    }

    private func makeDevice(platform: String, online: Bool) throws -> Device {
        let json = """
        {
            "device_id": "d1",
            "name": "Test",
            "platform": "\(platform)",
            "capabilities": ["clipboard"],
            "online": \(online),
            "last_seen": "2026-03-16T12:00:00Z"
        }
        """
        let decoder = JSONDecoder()
        decoder.dateDecodingStrategy = .tgSpringISO8601
        return try decoder.decode(Device.self, from: Data(json.utf8))
    }

    func testDeviceLabels() throws {
        XCTAssertEqual(try makeDevice(platform: "ios", online: true).platformLabel, "iOS")
        XCTAssertEqual(try makeDevice(platform: "darwin", online: true).platformLabel, "Mac")
        XCTAssertEqual(try makeDevice(platform: "android", online: false).statusLabel, "Offline")
    }
}
