import XCTest
@testable import TGClipboardKit

final class ClipCacheTests: XCTestCase {
    func testRetentionKeepsNewestTwentyWithinTwentyFourHours() {
        let now = Date(timeIntervalSince1970: 2_000_000_000)
        var clips = (1...25).map { index in
            makeClip(
                seq: UInt64(index),
                createdAt: now.addingTimeInterval(TimeInterval(index - 25) * 60)
            )
        }
        clips.append(makeClip(seq: 26, createdAt: now.addingTimeInterval(-25 * 60 * 60)))

        let retained = ClipCache.retained(clips, now: now)

        XCTAssertEqual(retained.count, 20)
        XCTAssertEqual(retained.first?.seq, 25)
        XCTAssertEqual(retained.last?.seq, 6)
        XCTAssertFalse(retained.contains { $0.seq == 26 })
    }

    func testRetentionDeduplicatesSequence() {
        let now = Date(timeIntervalSince1970: 2_000_000_000)
        let duplicate = makeClip(seq: 2, createdAt: now)
        let retained = ClipCache.retained([
            duplicate,
            makeClip(seq: 1, createdAt: now.addingTimeInterval(-60)),
            duplicate,
        ], now: now)

        XCTAssertEqual(retained.map(\.seq), [2, 1])
    }

    private func makeClip(seq: UInt64, createdAt: Date) -> ClipItem {
        ClipItem(
            seq: seq,
            content: "clip \(seq)",
            hash: "hash-\(seq)",
            source: "test",
            deviceID: "device",
            createdAt: createdAt,
            expiresAt: createdAt.addingTimeInterval(24 * 60 * 60)
        )
    }
}
