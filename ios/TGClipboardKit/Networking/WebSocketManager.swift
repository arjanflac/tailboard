import Foundation

/// Manages a WebSocket connection to the hub's /api/clip/stream endpoint.
/// Reconnects with exponential backoff. Container app only.
@Observable
public final class WebSocketManager: @unchecked Sendable {
    private var task: URLSessionWebSocketTask?
    private var connectionTask: Task<Void, Never>?
    private let session: URLSession
    private var lastSeq: UInt64 = 0

    public var baseURL: URL?
    public var onUpdate: ((ClipItem) -> Void)?
    public var onTransfer: ((Transfer) -> Void)?
    /// Called on every connection state transition. Listeners that touch UI
    /// state must hop to the main actor (transitions fire from the connect loop).
    public var onConnectionStateChange: ((ConnectionState) -> Void)?
    public var deviceID: String?
    public var connectionState: ConnectionState = .disconnected(reason: "Not started") {
        didSet { onConnectionStateChange?(connectionState) }
    }

    public init(session: URLSession = .shared) {
        self.session = session
    }

    public func start() {
        guard connectionTask == nil else { return }
        connectionTask = Task { [weak self] in
            await self?.connectLoop()
        }
    }

    public func stop() {
        connectionTask?.cancel()
        connectionTask = nil
        task?.cancel(with: .goingAway, reason: nil)
        task = nil
        connectionState = .disconnected(reason: "Stopped")
    }

    private func connectLoop() async {
        var backoff: UInt64 = 1_000_000_000

        while !Task.isCancelled {
            do {
                connectionState = .connecting
                try await connect()
                backoff = 1_000_000_000
            } catch {
                if Task.isCancelled { return }
                connectionState = ConnectionState.from(error: error, hubURL: baseURL)
                try? await Task.sleep(nanoseconds: backoff)
                backoff = min(backoff * 2, 60_000_000_000)
            }
        }
    }

    private func connect() async throws {
        guard let baseURL else { throw TGClipboardError.noHubURL }

        var urlString = baseURL.appendingPathComponent("/api/clip/stream").absoluteString
        if urlString.hasPrefix("https") {
            urlString = "wss" + urlString.dropFirst(5)
        } else if urlString.hasPrefix("http") {
            urlString = "ws" + urlString.dropFirst(4)
        }
        guard var components = URLComponents(string: urlString) else { return }
        var queryItems: [URLQueryItem] = []
        if lastSeq > 0 {
            queryItems.append(URLQueryItem(name: "since_seq", value: String(lastSeq)))
        }
        if let deviceID {
            queryItems.append(URLQueryItem(name: "device_id", value: deviceID))
        }
        components.queryItems = queryItems.isEmpty ? nil : queryItems
        guard let url = components.url else { return }
        let wsTask = session.webSocketTask(with: url)
        self.task = wsTask
        defer {
            wsTask.cancel(with: .goingAway, reason: nil)
            if self.task === wsTask {
                self.task = nil
            }
        }
        wsTask.resume()

        let decoder = JSONDecoder()
        decoder.dateDecodingStrategy = .tgSpringISO8601
        var receivedServerMessage = false

        while !Task.isCancelled {
            let message = try await wsTask.receive()
            let data: Data
            switch message {
            case .string(let text): data = Data(text.utf8)
            case .data(let d): data = d
            @unknown default: continue
            }

            guard let wsMsg = try? decoder.decode(WSMessage.self, from: data) else { continue }
            if !receivedServerMessage {
                receivedServerMessage = true
                connectionState = .connected
            }
            if wsMsg.type == "clip_update", let item = wsMsg.item {
                lastSeq = item.seq
                onUpdate?(item)
            } else if (wsMsg.type == "transfer_offer" || wsMsg.type == "transfer_state"),
                      let transfer = wsMsg.transfer {
                onTransfer?(transfer)
            }
        }
    }
}
