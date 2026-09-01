import Foundation
import TGClipboardKit

struct ControlState: Decodable {
    let deviceID: String
    let nodeName: String?
    let hubURL: String?
    let paused: Bool
    let connection: String?
    let clip: ClipItem?
    let clipSize: Int64?
    let devices: [Device]?

    enum CodingKeys: String, CodingKey {
        case deviceID = "device_id"
        case nodeName = "node_name"
        case hubURL = "hub_url"
        case paused, connection, clip, devices
        case clipSize = "clip_size"
    }
}

enum ControlError: Error, LocalizedError {
    case engineNotRunning
    case requestFailed(String)

    var errorDescription: String? {
        switch self {
        case .engineNotRunning: return "Tailboard Engine isn't running"
        case .requestFailed(let message): return message
        }
    }
}

actor ControlClient {
    private let baseURL: URL
    private let session: URLSession
    private let decoder: JSONDecoder

    init(port: Int = 9438) {
        self.baseURL = URL(string: "http://127.0.0.1:\(port)")!
        let config = URLSessionConfiguration.ephemeral
        config.timeoutIntervalForRequest = 5
        self.session = URLSession(configuration: config)
        self.decoder = JSONDecoder()
        decoder.dateDecodingStrategy = .custom { decoder in
            let fractional = ISO8601DateFormatter()
            fractional.formatOptions = [.withInternetDateTime, .withFractionalSeconds]
            let plain = ISO8601DateFormatter()
            let value = try decoder.singleValueContainer().decode(String.self)
            if let date = fractional.date(from: value) ?? plain.date(from: value) {
                return date
            }
            throw DecodingError.dataCorrupted(.init(
                codingPath: decoder.codingPath,
                debugDescription: "Unparseable date: \(value)"
            ))
        }
    }

    func state() async throws -> ControlState {
        let request = URLRequest(url: baseURL.appendingPathComponent("/api/state"))
        let (data, _) = try await perform(request, expectSuccess: true)
        return try decoder.decode(ControlState.self, from: data)
    }

    func setPaused(_ paused: Bool) async throws {
        var request = URLRequest(url: baseURL.appendingPathComponent("/api/pause"))
        request.httpMethod = "POST"
        request.setValue("application/json", forHTTPHeaderField: "Content-Type")
        request.httpBody = try JSONSerialization.data(withJSONObject: ["paused": paused])
        _ = try await perform(request, expectSuccess: true)
    }

    private func perform(
        _ request: URLRequest,
        expectSuccess: Bool
    ) async throws -> (Data, URLResponse) {
        let data: Data
        let response: URLResponse
        do {
            (data, response) = try await session.data(for: request)
        } catch {
            throw ControlError.engineNotRunning
        }
        if expectSuccess, let http = response as? HTTPURLResponse,
           !(200...299).contains(http.statusCode) {
            let message = (try? JSONDecoder().decode([String: String].self, from: data))?["error"]
            throw ControlError.requestFailed(message ?? "Request failed (\(http.statusCode))")
        }
        return (data, response)
    }
}
