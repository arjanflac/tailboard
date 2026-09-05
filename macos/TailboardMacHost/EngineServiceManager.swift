import AppKit
import CoreServices
import CryptoKit
import Foundation
import ServiceManagement

enum EngineServiceError: LocalizedError {
    case embeddedEngineMissing
    case approvalRequired
    case registrationFailed(String)

    var errorDescription: String? {
        switch self {
        case .embeddedEngineMissing:
            return "Tailboard.app is missing its embedded engine"
        case .approvalRequired:
            return "Allow Tailboard in System Settings → General → Login Items"
        case .registrationFailed(let message):
            return "Tailboard Engine could not start: \(message)"
        }
    }
}

/// Registers and updates Tailboard's only persistent Mac process.
@MainActor
final class EngineServiceManager {
    static let shared = EngineServiceManager()

    private static let engineBundleIdentifier = "com.arjanflac.tailboard.engine.background"
    private static let registeredHashKey = "TailboardRegisteredEngineBackgroundSHA256"
    static let lastErrorKey = "TailboardLastEngineServiceError"

    private let service = SMAppService.loginItem(identifier: engineBundleIdentifier)

    private init() {}

    func activate() async throws {
        registerBundleLocations()
        try await ensureEngineRegistered()
        UserDefaults.standard.removeObject(forKey: Self.lastErrorKey)
    }

    func openLoginItemsSettings() {
        SMAppService.openSystemSettingsLoginItems()
    }

    private func ensureEngineRegistered() async throws {
        let engineAppURL = Bundle.main.bundleURL
            .appendingPathComponent("Contents/Library/LoginItems", isDirectory: true)
            .appendingPathComponent("Tailboard Engine.app", isDirectory: true)
        let engineURL = engineAppURL.appendingPathComponent("Contents/MacOS/Tailboard Engine")
        guard FileManager.default.isExecutableFile(atPath: engineURL.path) else {
            throw EngineServiceError.embeddedEngineMissing
        }

        var registrationContent = try Data(contentsOf: engineURL)
        registrationContent.append(
            try Data(contentsOf: engineAppURL.appendingPathComponent("Contents/Info.plist"))
        )
        registrationContent.append(Data((Bundle.main.object(
            forInfoDictionaryKey: "CFBundleVersion"
        ) as? String ?? "").utf8))
        let engineHash = SHA256.hash(data: registrationContent)
            .map { String(format: "%02x", $0) }
            .joined()
        let storedHash = UserDefaults.standard.string(forKey: Self.registeredHashKey)

        switch service.status {
        case .requiresApproval:
            throw EngineServiceError.approvalRequired
        case .enabled:
            if storedHash != engineHash {
                // ServiceManagement keeps the parent bundle version captured
                // at registration time. Refresh that record after an in-place
                // app update so launchd executes the replacement helper.
                try await service.unregister()
                try await Task.sleep(for: .milliseconds(300))
                try service.register()
                UserDefaults.standard.set(engineHash, forKey: Self.registeredHashKey)
                return
            }
            let isRunning = !NSRunningApplication.runningApplications(
                withBundleIdentifier: Self.engineBundleIdentifier
            ).isEmpty
            if !isRunning {
                try await restartEngineApplication(at: engineAppURL)
                UserDefaults.standard.set(engineHash, forKey: Self.registeredHashKey)
            }
            return
        case .notRegistered, .notFound:
            break
        @unknown default:
            break
        }

        var lastRegistrationError: Error?
        for attempt in 0..<8 {
            do {
                try service.register()
                UserDefaults.standard.set(engineHash, forKey: Self.registeredHashKey)
                return
            } catch {
                if service.status == .enabled {
                    UserDefaults.standard.set(engineHash, forKey: Self.registeredHashKey)
                    return
                }
                if service.status == .requiresApproval {
                    throw EngineServiceError.approvalRequired
                }
                lastRegistrationError = error
                guard attempt < 7 else { break }
                try await Task.sleep(for: .milliseconds(250 * (attempt + 1)))
            }
        }

        throw EngineServiceError.registrationFailed(
            lastRegistrationError?.localizedDescription ?? "unknown registration error"
        )
    }

    private func registerBundleLocations() {
        let appURL = Bundle.main.bundleURL
        let engineURL = appURL
            .appendingPathComponent("Contents/Library/LoginItems", isDirectory: true)
            .appendingPathComponent("Tailboard Engine.app", isDirectory: true)
        LSRegisterURL(appURL as CFURL, true)
        LSRegisterURL(engineURL as CFURL, true)
    }

    /// Restart the already-approved nested login item in place after updates.
    private func restartEngineApplication(at url: URL) async throws {
        let runningApplications = NSRunningApplication.runningApplications(
            withBundleIdentifier: Self.engineBundleIdentifier
        )
        for application in runningApplications {
            application.terminate()
        }
        if !runningApplications.isEmpty {
            try await Task.sleep(for: .milliseconds(750))
        }
        for application in runningApplications where !application.isTerminated {
            application.forceTerminate()
        }

        // On macOS 27 betas an enabled SMAppService can remain registered after
        // an in-place bundle update while its previous process is recorded as a
        // successful exit. Launch Services may acknowledge the first open
        // request before the new executable is actually running, so verify and
        // retry instead of silently leaving the relay stopped.
        for attempt in 0..<4 {
            try await requestEngineLaunch(at: url)
            if await waitForEngineLaunch() {
                return
            }
            if attempt < 3 {
                try await Task.sleep(for: .milliseconds(500 * (attempt + 1)))
            }
        }

        throw EngineServiceError.registrationFailed(
            "macOS accepted the launch request but the engine did not remain running"
        )
    }

    private func requestEngineLaunch(at url: URL) async throws {
        let configuration = NSWorkspace.OpenConfiguration()
        configuration.activates = false
        try await withCheckedThrowingContinuation {
            (continuation: CheckedContinuation<Void, Error>) in
            NSWorkspace.shared.openApplication(at: url, configuration: configuration) { _, error in
                if let error {
                    continuation.resume(throwing: error)
                    return
                }
                continuation.resume()
            }
        }
    }

    private func waitForEngineLaunch() async -> Bool {
        for _ in 0..<12 {
            if !NSRunningApplication.runningApplications(
                withBundleIdentifier: Self.engineBundleIdentifier
            ).isEmpty {
                return true
            }
            try? await Task.sleep(for: .milliseconds(250))
        }
        return false
    }

}
