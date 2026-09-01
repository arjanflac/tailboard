import XCTest
@testable import TGClipboardKit

final class ClipItemTests: XCTestCase {
    private let decoder: JSONDecoder = {
        let decoder = JSONDecoder()
        decoder.dateDecodingStrategy = .tgSpringISO8601
        return decoder
    }()

    func testJSONMatchesTextOnlyHubProtocol() throws {
        let json = """
        {
            "seq": 42,
            "content": "hello world",
            "hash": "abc",
            "source": "mac",
            "created_at": "2026-03-16T12:00:00Z",
            "expires_at": "2026-03-17T12:00:00Z"
        }
        """
        let item = try decoder.decode(ClipItem.self, from: Data(json.utf8))
        XCTAssertEqual(item.seq, 42)
        XCTAssertEqual(item.content, "hello world")
        XCTAssertEqual(item.preview, "hello world")
        XCTAssertEqual(item.displaySummary, "Text")
    }

    func testSHA256MatchesGo() {
        XCTAssertEqual(
            ClipHash.sha256Hex("hello"),
            "2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824"
        )
    }

    func testWebSocketMessageDecode() throws {
        let json = """
        {
            "type": "clip_update",
            "item": {
                "seq": 5,
                "content": "test",
                "hash": "abc",
                "source": "pixel",
                "created_at": "2026-03-16T12:00:00Z",
                "expires_at": "2026-03-17T12:00:00Z"
            }
        }
        """
        let message = try decoder.decode(WSMessage.self, from: Data(json.utf8))
        XCTAssertEqual(message.type, "clip_update")
        XCTAssertEqual(message.item?.content, "test")
    }
}
