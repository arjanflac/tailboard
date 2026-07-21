import Foundation

public final class BackgroundUploadCoordinator: NSObject, URLSessionDelegate, URLSessionTaskDelegate, @unchecked Sendable {
    public static let shared = BackgroundUploadCoordinator()
    public static let sessionIdentifier = "com.thalys.tgclipboard.transfer-uploads"

    private var completionHandler: (() -> Void)?
    private lazy var session: URLSession = makeSession()

    public func schedule(request: URLRequest, fileURL: URL) {
        let task = session.uploadTask(with: request, fromFile: fileURL)
        task.taskDescription = fileURL.path
        task.resume()
    }

    public func reconnect(completionHandler: @escaping () -> Void) {
        self.completionHandler = completionHandler
        session = makeSession()
    }

    public func urlSession(
        _ session: URLSession,
        task: URLSessionTask,
        didCompleteWithError error: Error?
    ) {
        if let path = task.taskDescription {
            try? FileManager.default.removeItem(atPath: path)
        }
    }

    public func urlSessionDidFinishEvents(forBackgroundURLSession session: URLSession) {
        DispatchQueue.main.async {
            self.completionHandler?()
            self.completionHandler = nil
        }
    }

    private func makeSession() -> URLSession {
        let configuration = URLSessionConfiguration.background(
            withIdentifier: Self.sessionIdentifier
        )
        configuration.sharedContainerIdentifier = AppGroupStore.suiteName
        configuration.sessionSendsLaunchEvents = true
        configuration.isDiscretionary = false
        return URLSession(
            configuration: configuration,
            delegate: self,
            delegateQueue: nil
        )
    }
}
