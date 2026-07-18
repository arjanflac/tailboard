import ActivityKit
import Foundation

public struct ClipHubActivityAttributes: ActivityAttributes {
    public struct ContentState: Codable, Hashable {
        public let preview: String
        public let source: String
        public let updatedAt: Date

        public init(preview: String, source: String, updatedAt: Date) {
            self.preview = preview
            self.source = source
            self.updatedAt = updatedAt
        }
    }

    public let title: String

    public init(title: String = "Current Clip") {
        self.title = title
    }
}
