import AppKit
import CoreServices
import CryptoKit
import Darwin
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
        if !(try await waitForEngineLaunch()) {
            // Refresh an approved job that macOS retained without a process.
            // Launch through ServiceManagement so only the login item owns it.
            try await service.unregister()
            try await Task.sleep(for: .milliseconds(300))
            try service.register()
            guard try await waitForEngineLaunch() else {
                if service.status == .requiresApproval {
                    throw EngineServiceError.approvalRequired
                }
                throw EngineServiceError.registrationFailed(
                    "the background service did not remain running; reopen Tailboard to retry"
                )
            }
        }
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

    // A Go login item launched by launchd may never appear in AppKit's
    // runningApplications list. Match the actual executable path instead.
    private func engineIsRunning() -> Bool {
        let expectedPath = Bundle.main.bundleURL
            .appendingPathComponent("Contents/Library/LoginItems/Tailboard Engine.app/Contents/MacOS/Tailboard Engine")
            .path
        let capacity = Int(proc_listallpids(nil, 0)) + 64
        guard capacity > 64 else { return false }
        var pids = [pid_t](repeating: 0, count: capacity)
        let count = proc_listallpids(&pids, Int32(capacity * MemoryLayout<pid_t>.stride))
        for pid in pids.prefix(max(0, min(Int(count), capacity))) where pid > 0 {
            // PROC_PIDPATHINFO_MAXSIZE is 4 * MAXPATHLEN (not imported by Swift).
            var path = [CChar](repeating: 0, count: 4 * Int(MAXPATHLEN))
            if proc_pidpath(pid, &path, UInt32(path.count)) > 0,
               String(cString: path) == expectedPath {
                return true
            }
        }
        return false
    }

    private func waitForEngineLaunch() async throws -> Bool {
        var consecutiveChecks = 0
        for _ in 0..<24 {
            consecutiveChecks = engineIsRunning() ? consecutiveChecks + 1 : 0
            if consecutiveChecks >= 4 { return true }
            try await Task.sleep(for: .milliseconds(250))
        }
        return false
    }
}
