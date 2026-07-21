import Foundation
import TGClipboardKit

/// State returned by tg-clipd's loopback control API (`GET /api/state`).
struct ControlState: Decodable {
    let deviceID: String
    let nodeName: String?
    let hubURL: String?
    let paused: Bool
    /// "synced", "offline", or "no-tailscale" — tg-clipd's view of the hub.
    let connection: String?
    let clip: ClipItem?
    let clipSize: Int64?
    let devices: [Device]?
    let transfers: [Transfer]?

    enum CodingKeys: String, CodingKey {
        case deviceID = "device_id"
        case nodeName = "node_name"
        case hubURL = "hub_url"
        case paused, connection, clip, devices, transfers
        case clipSize = "clip_size"
    }
}

enum ControlError: Error, LocalizedError {
    case engineNotRunning
    case requestFailed(String)

    var errorDescription: String? {
        switch self {
        case .engineNotRunning: return "tg-clipboard engine isn't running"
        case .requestFailed(let message): return message
        }
    }
}

/// Pure client of tg-clipd's loopback control surface (127.0.0.1:9438).
/// The app stays dumb: tg-clipd owns clipboard sync, transfers, and hub talk.
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
        // Go emits RFC3339Nano; plain .iso8601 rejects fractional seconds.
        let fractional = ISO8601DateFormatter()
        fractional.formatOptions = [.withInternetDateTime, .withFractionalSeconds]
        let plain = ISO8601DateFormatter()
        decoder.dateDecodingStrategy = .custom { decoder in
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
        let (data, _) = try await get("/api/state")
        return try decoder.decode(ControlState.self, from: data)
    }

    func setPaused(_ paused: Bool) async throws {
        _ = try await post("/api/pause", json: ["paused": paused])
    }

    func transferAction(id: String, action: String) async throws {
        _ = try await post("/api/transfers/\(id)/\(action)")
    }

    func revealDownloads() async throws {
        _ = try await post("/api/reveal")
    }

    /// Uploads files to a device via multipart `POST /api/send?to=<id>`.
    /// tg-clipd stages, hashes, and chunk-uploads them to the hub.
    func send(fileURLs: [URL], to deviceID: String) async throws {
        var components = URLComponents(url: baseURL.appendingPathComponent("/api/send"), resolvingAgainstBaseURL: false)!
        components.queryItems = [URLQueryItem(name: "to", value: deviceID)]
        var request = URLRequest(url: components.url!)
        request.httpMethod = "POST"
        request.timeoutInterval = 3600

        let boundary = "tg-clipboard-\(UUID().uuidString)"
        request.setValue("multipart/form-data; boundary=\(boundary)", forHTTPHeaderField: "Content-Type")

        var body = Data()
        for fileURL in fileURLs {
            let name = fileURL.lastPathComponent
            let fileData = try Data(contentsOf: fileURL)
            body.append(Data("--\(boundary)\r\n".utf8))
            body.append(Data("Content-Disposition: form-data; name=\"files\"; filename=\"\(quoteEscaped(name))\"\r\n".utf8))
            body.append(Data("Content-Type: application/octet-stream\r\n\r\n".utf8))
            body.append(fileData)
            body.append(Data("\r\n".utf8))
        }
        body.append(Data("--\(boundary)--\r\n".utf8))
        request.httpBody = body

        let (data, response) = try await perform(request)
        guard let http = response as? HTTPURLResponse, (200...299).contains(http.statusCode) else {
            throw ControlError.requestFailed(Self.serverMessage(data) ?? "Send failed")
        }
    }

    // MARK: - Plumbing

    private func get(_ path: String) async throws -> (Data, URLResponse) {
        let request = URLRequest(url: baseURL.appendingPathComponent(path))
        return try await perform(request, expectSuccess: true)
    }

    private func post(_ path: String, json: [String: Bool]? = nil) async throws -> (Data, URLResponse) {
        var request = URLRequest(url: baseURL.appendingPathComponent(path))
        request.httpMethod = "POST"
        if let json {
            request.setValue("application/json", forHTTPHeaderField: "Content-Type")
            request.httpBody = try JSONSerialization.data(withJSONObject: json)
        }
        return try await perform(request, expectSuccess: true)
    }

    private func perform(_ request: URLRequest, expectSuccess: Bool = false) async throws -> (Data, URLResponse) {
        let data: Data
        let response: URLResponse
        do {
            (data, response) = try await session.data(for: request)
        } catch {
            throw ControlError.engineNotRunning
        }
        if expectSuccess, let http = response as? HTTPURLResponse,
           !(200...299).contains(http.statusCode) {
            throw ControlError.requestFailed(Self.serverMessage(data) ?? "Request failed (\(http.statusCode))")
        }
        return (data, response)
    }

    private static func serverMessage(_ data: Data) -> String? {
        (try? JSONDecoder().decode([String: String].self, from: data))?["error"]
    }

    private func quoteEscaped(_ value: String) -> String {
        value.replacingOccurrences(of: "\\", with: "_")
            .replacingOccurrences(of: "\"", with: "_")
    }
}
