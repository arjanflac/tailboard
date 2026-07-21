import XCTest
@testable import TGClipboardKit

/// Tests for the humanized display helpers added for the keyboard, share
/// extension, and widget redesign (ios/UX_AUDIT_EXTENSIONS.md).
final class DisplayHelperTests: XCTestCase {

    private func makeClip(mimeType: String, content: String? = nil, base64Data: String? = nil) throws -> ClipItem {
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
        if let base64Data {
            json += ",\n    \"data\": \"\(base64Data)\""
        }
        json += "\n}"
        let decoder = JSONDecoder()
        decoder.dateDecodingStrategy = .tgSpringISO8601
        return try decoder.decode(ClipItem.self, from: Data(json.utf8))
    }

    // MARK: - displaySummary

    func testPlainTextSummary() throws {
        let clip = try makeClip(mimeType: "text/plain", content: "hello world")
        XCTAssertEqual(clip.displaySummary, "Text")
        XCTAssertFalse(clip.isLink)
    }

    func testLinkSummary() throws {
        let clip = try makeClip(mimeType: "text/plain", content: "https://example.com/path?q=1")
        XCTAssertEqual(clip.displaySummary, "Link")
        XCTAssertTrue(clip.isLink)
    }

    func testLinkRequiresScheme() throws {
        let clip = try makeClip(mimeType: "text/plain", content: "example.com/path")
        XCTAssertFalse(clip.isLink)
        XCTAssertEqual(clip.displaySummary, "Text")
    }

    func testLinkRejectsWhitespace() throws {
        let clip = try makeClip(mimeType: "text/plain", content: "see https://example.com")
        XCTAssertFalse(clip.isLink)
        XCTAssertEqual(clip.displaySummary, "Text")
    }

    func testImageSummary() throws {
        // 8 bytes of PNG header prefix.
        let clip = try makeClip(mimeType: "image/png", base64Data: "iVBORw0KGgo=")
        XCTAssertTrue(clip.displaySummary.hasPrefix("Image · "))
        XCTAssertFalse(clip.displaySummary.contains("image/png"))
        XCTAssertFalse(clip.displaySummary.contains("bytes]"))
    }

    func testGenericFileSummaryHasSizeAndNoMIME() throws {
        let clip = try makeClip(mimeType: "application/pdf", base64Data: "JVBERi0xLjQK")
        XCTAssertTrue(clip.displaySummary.contains(" · "))
        XCTAssertFalse(clip.displaySummary.contains("application/pdf"))
    }

    func testFormattedByteCount() throws {
        let clip = try makeClip(mimeType: "image/png", base64Data: "iVBORw0KGgo=")
        XCTAssertEqual(clip.formattedByteCount, ByteCountFormatter.string(fromByteCount: 8, countStyle: .file))
    }

    func testStaticSummaryMatchesInstance() throws {
        let clip = try makeClip(mimeType: "image/png", base64Data: "iVBORw0KGgo=")
        XCTAssertEqual(
            ClipItem.displaySummary(mimeType: "image/png", byteCount: 8, textContent: nil),
            clip.displaySummary
        )
    }

    // MARK: - Device helpers

    private func makeDevice(platform: String, online: Bool) throws -> Device {
        let json = """
        {
            "device_id": "d1",
            "name": "Test",
            "platform": "\(platform)",
            "capabilities": ["transfers"],
            "online": \(online),
            "last_seen": "2026-03-16T12:00:00Z"
        }
        """.data(using: .utf8)!
        let decoder = JSONDecoder()
        decoder.dateDecodingStrategy = .tgSpringISO8601
        return try decoder.decode(Device.self, from: json)
    }

    func testPlatformSymbolsMatchMainApp() throws {
        XCTAssertEqual(try makeDevice(platform: "ios", online: true).platformSymbol, "iphone")
        XCTAssertEqual(try makeDevice(platform: "darwin", online: true).platformSymbol, "laptopcomputer")
        XCTAssertEqual(try makeDevice(platform: "windows", online: true).platformSymbol, "desktopcomputer")
        XCTAssertEqual(try makeDevice(platform: "linux", online: true).platformSymbol, "terminal")
        XCTAssertEqual(try makeDevice(platform: "plan9", online: true).platformSymbol, "display")
    }

    func testStatusLabel() throws {
        XCTAssertEqual(try makeDevice(platform: "ios", online: true).statusLabel, "Online")
        XCTAssertEqual(try makeDevice(platform: "ios", online: false).statusLabel, "Offline")
    }
}
