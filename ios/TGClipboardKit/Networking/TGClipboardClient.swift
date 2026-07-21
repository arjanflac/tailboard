import Foundation

public enum TGClipboardError: Error, LocalizedError {
    case noHubURL
    case hubUnreachable(underlying: Error)
    case httpError(statusCode: Int, body: String)
    case emptyClipboard
    case decodingError(Error)

    public var errorDescription: String? {
        switch self {
        case .noHubURL: return "Hub URL not configured"
        case .hubUnreachable(let e): return "Hub unreachable: \(e.localizedDescription)"
        case .httpError(let code, let body): return "HTTP \(code): \(body)"
        case .emptyClipboard: return "Clipboard is empty on the hub"
        case .decodingError(let e): return "Decode error: \(e.localizedDescription)"
        }
    }
}

/// Async REST client for the TGClipboard API.
public actor TGClipboardClient {
    private let session: URLSession
    private let decoder: JSONDecoder
    public var baseURL: URL?
    public var sourceName: String

    public init(baseURL: URL? = nil, sourceName: String = "iphone") {
        self.baseURL = baseURL
        self.sourceName = sourceName

        let config = URLSessionConfiguration.default
        config.timeoutIntervalForRequest = 10
        config.timeoutIntervalForResource = 30
        config.waitsForConnectivity = true
        self.session = URLSession(configuration: config)

        self.decoder = JSONDecoder()
        self.decoder.dateDecodingStrategy = .tgSpringISO8601
    }

    // MARK: - GET /api/clip

    public func getCurrentClip() async throws -> ClipItem? {
        let (data, response) = try await get("/api/clip")
        if response.statusCode == 204 { return nil }
        try validate(response, body: data)
        return try decoder.decode(ClipItem.self, from: data)
    }

    // MARK: - GET /api/clip/history

    public func getHistory(limit: Int = 50) async throws -> [ClipItem] {
        let (data, response) = try await get("/api/clip/history?limit=\(limit)")
        try validate(response, body: data)
        return try decoder.decode([ClipItem].self, from: data)
    }

    // MARK: - POST /api/clip (text)

    public func postClip(content: String, mimeType: String = "text/plain") async throws -> ClipItem {
        let body: [String: Any] = ["content": content, "mime_type": mimeType]
        return try await post("/api/clip", json: body)
    }

    // MARK: - POST /api/clip (binary)

    public func postClip(data: Data, mimeType: String) async throws -> ClipItem {
        let url = try resolveURL("/api/clip/blob")
        var request = URLRequest(url: url)
        request.httpMethod = "POST"
        request.setValue(mimeType, forHTTPHeaderField: "Content-Type")
        request.setValue(sourceName, forHTTPHeaderField: "X-Clip-Source")
        request.httpBody = data
        let (responseData, response) = try await perform(request)
        try validate(response, body: responseData)
        guard let item = try await getCurrentClip() else {
            throw TGClipboardError.emptyClipboard
        }
        return item
    }

    // MARK: - GET /api/status

    public func getStatus() async throws -> HubStatus {
        let (data, response) = try await get("/api/status")
        try validate(response, body: data)
        return try decoder.decode(HubStatus.self, from: data)
    }

    public func registerDevice(deviceID: String, name: String) async throws -> Device {
        let body: [String: Any] = [
            "device_id": deviceID,
            "name": name,
            "platform": "ios",
            "capabilities": ["clipboard", "transfers"]
        ]
        let (data, response) = try await postJSON("/api/devices/register", body: body)
        try validate(response, body: data)
        return try decoder.decode(Device.self, from: data)
    }

    public func getDevices() async throws -> [Device] {
        let (data, response) = try await get("/api/devices")
        try validate(response, body: data)
        return try decoder.decode([Device].self, from: data)
    }

    /// Removes a device from the roster (spec: long-press → Remove device).
    public func removeDevice(deviceID: String) async throws {
        let (data, response) = try await delete("/api/devices/\(deviceID)")
        try validate(response, body: data)
    }

    public func getTransfers(deviceID: String, role: String? = nil, state: String? = nil) async throws -> [Transfer] {
        var components = URLComponents()
        var items = [URLQueryItem(name: "device_id", value: deviceID)]
        if let role { items.append(URLQueryItem(name: "role", value: role)) }
        if let state { items.append(URLQueryItem(name: "state", value: state)) }
        components.queryItems = items
        let (data, response) = try await get("/api/transfers?\(components.percentEncodedQuery ?? "")")
        try validate(response, body: data)
        // Older hubs serialize an empty roster as JSON null (Go nil slice).
        if data == Data("null".utf8) || data == Data("null\n".utf8) { return [] }
        return try decoder.decode([Transfer].self, from: data)
    }

    public func createTransfer(
        deviceID: String,
        toDevice: String,
        fileName: String,
        size: Int64,
        mimeType: String,
        sha256: String
    ) async throws -> CreateTransferResponse {
        let body: [String: Any] = [
            "to_device": toDevice,
            "files": [[
                "name": fileName,
                "size": size,
                "mime": mimeType,
                "sha256": sha256
            ]]
        ]
        let url = try resolveURL("/api/transfers")
        var request = URLRequest(url: url)
        request.httpMethod = "POST"
        request.setValue("application/json", forHTTPHeaderField: "Content-Type")
        request.setValue(deviceID, forHTTPHeaderField: "X-Clip-Device-ID")
        request.httpBody = try JSONSerialization.data(withJSONObject: body)
        let (data, response) = try await perform(request)
        try validate(response, body: data)
        return try decoder.decode(CreateTransferResponse.self, from: data)
    }

    public func scheduleTransferUpload(
        deviceID: String,
        toDevice: String,
        data: Data,
        fileName: String,
        mimeType: String
    ) async throws -> Transfer {
        let safeName = URL(fileURLWithPath: fileName).lastPathComponent
        let created = try await createTransfer(
            deviceID: deviceID,
            toDevice: toDevice,
            fileName: safeName,
            size: Int64(data.count),
            mimeType: mimeType,
            sha256: ClipHash.sha256Hex(data)
        )
        guard let uploadPath = created.uploadURLs.first,
              let uploadURL = try? resolveURL(uploadPath),
              let container = FileManager.default.containerURL(
                forSecurityApplicationGroupIdentifier: AppGroupStore.suiteName
              ) else {
            throw TGClipboardError.noHubURL
        }
        let directory = container.appendingPathComponent("BackgroundUploads", isDirectory: true)
        try FileManager.default.createDirectory(at: directory, withIntermediateDirectories: true)
        let staged = directory.appendingPathComponent("\(created.transfer.transferID)-0.upload")
        try data.write(to: staged, options: .atomic)

        var request = URLRequest(url: uploadURL)
        request.httpMethod = "PUT"
        request.setValue(deviceID, forHTTPHeaderField: "X-Clip-Device-ID")
        request.setValue(mimeType, forHTTPHeaderField: "Content-Type")
        request.setValue("bytes 0-*/\(data.count)", forHTTPHeaderField: "Content-Range")
        BackgroundUploadCoordinator.shared.schedule(request: request, fileURL: staged)
        return created.transfer
    }

    public func transferAction(deviceID: String, transferID: String, action: String) async throws -> Transfer {
        let url = try resolveURL("/api/transfers/\(transferID)/\(action)")
        var request = URLRequest(url: url)
        request.httpMethod = "POST"
        request.setValue(deviceID, forHTTPHeaderField: "X-Clip-Device-ID")
        let (data, response) = try await perform(request)
        try validate(response, body: data)
        return try decoder.decode(Transfer.self, from: data)
    }

    public func downloadTransferFile(
        deviceID: String,
        transferID: String,
        index: Int,
        destination: URL
    ) async throws {
        let url = try resolveURL("/api/transfers/\(transferID)/files/\(index)")
        var request = URLRequest(url: url)
        request.setValue(deviceID, forHTTPHeaderField: "X-Clip-Device-ID")
        do {
            let (temporary, response) = try await session.download(for: request)
            guard let http = response as? HTTPURLResponse else {
                throw TGClipboardError.hubUnreachable(underlying: URLError(.badServerResponse))
            }
            if !(200..<300).contains(http.statusCode) {
                let body = (try? Data(contentsOf: temporary)) ?? Data()
                try validate(http, body: body)
            }
            try FileManager.default.moveItem(at: temporary, to: destination)
        } catch let error as TGClipboardError {
            throw error
        } catch {
            throw TGClipboardError.hubUnreachable(underlying: error)
        }
    }

    // MARK: - Probe

    public func probe() async -> Bool {
        do {
            _ = try await getStatus()
            return true
        } catch {
            return false
        }
    }

    // MARK: - Helpers

    private func resolveURL(_ path: String) throws -> URL {
        guard let baseURL else { throw TGClipboardError.noHubURL }
        guard let url = URL(string: path, relativeTo: baseURL)?.absoluteURL else {
            throw TGClipboardError.noHubURL
        }
        return url
    }

    private func get(_ path: String) async throws -> (Data, HTTPURLResponse) {
        let url = try resolveURL(path)
        return try await perform(URLRequest(url: url))
    }

    private func delete(_ path: String) async throws -> (Data, HTTPURLResponse) {
        let url = try resolveURL(path)
        var request = URLRequest(url: url)
        request.httpMethod = "DELETE"
        return try await perform(request)
    }

    private func post(_ path: String, json body: [String: Any]) async throws -> ClipItem {
        let (data, response) = try await postJSON(path, body: body)
        try validate(response, body: data)
        return try decoder.decode(ClipItem.self, from: data)
    }

    private func postJSON(_ path: String, body: [String: Any]) async throws -> (Data, HTTPURLResponse) {
        let url = try resolveURL(path)
        var request = URLRequest(url: url)
        request.httpMethod = "POST"
        request.setValue("application/json", forHTTPHeaderField: "Content-Type")
        request.setValue(sourceName, forHTTPHeaderField: "X-Clip-Source")
        request.httpBody = try JSONSerialization.data(withJSONObject: body)

        return try await perform(request)
    }

    private func perform(_ request: URLRequest) async throws -> (Data, HTTPURLResponse) {
        do {
            let (data, response) = try await session.data(for: request)
            guard let http = response as? HTTPURLResponse else {
                throw TGClipboardError.hubUnreachable(underlying: URLError(.badServerResponse))
            }
            return (data, http)
        } catch let e as TGClipboardError {
            throw e
        } catch {
            throw TGClipboardError.hubUnreachable(underlying: error)
        }
    }

    private func validate(_ response: HTTPURLResponse, body: Data) throws {
        guard (200..<300).contains(response.statusCode) else {
            throw TGClipboardError.httpError(
                statusCode: response.statusCode,
                body: String(data: body, encoding: .utf8) ?? ""
            )
        }
    }
}
