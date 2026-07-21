import Foundation

/// The Go hub emits RFC3339Nano timestamps — fractional seconds plus a
/// numeric zone offset ("2026-07-21T15:00:47.923216-03:00"). Swift's plain
/// `.iso8601` strategy rejects fractional seconds, which silently broke
/// every decode of devices, clips, and stream messages. Every decoder that
/// touches hub JSON must use this strategy.
public extension JSONDecoder {
    static func tgSpring() -> JSONDecoder {
        let decoder = JSONDecoder()
        decoder.dateDecodingStrategy = .tgSpringISO8601
        return decoder
    }
}

public extension JSONDecoder.DateDecodingStrategy {
    static let tgSpringISO8601: JSONDecoder.DateDecodingStrategy = {
        let fractional = ISO8601DateFormatter()
        fractional.formatOptions = [.withInternetDateTime, .withFractionalSeconds]
        let plain = ISO8601DateFormatter()
        return .custom { decoder in
            let value = try decoder.singleValueContainer().decode(String.self)
            if let date = fractional.date(from: value) ?? plain.date(from: value) {
                return date
            }
            throw DecodingError.dataCorrupted(DecodingError.Context(
                codingPath: decoder.codingPath,
                debugDescription: "Unparseable RFC3339 date: \(value)"
            ))
        }
    }()
}
