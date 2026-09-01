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

/// Owns the modern macOS background registration for the bundled Go engine.
/// User-specific launch arguments live in Application Support so the signed
/// app bundle remains immutable and notarizable.
@MainActor
final class EngineServiceManager {
    private struct LegacyAgentBackup {
        let url: URL
        let data: Data
    }

    static let shared = EngineServiceManager()

    private static let engineBundleIdentifier = "com.arjanflac.tailboard.engine.background"
    private static let legacyServicePlistName = "com.arjanflac.tailboard.engine.plist"
    private static let legacyServiceLabel = "com.arjanflac.tailboard.engine"
    private static let registeredHashKey = "TailboardRegisteredEngineBackgroundSHA256"
    static let lastErrorKey = "TailboardLastEngineServiceError"
    private static let defaultArguments = ["--embed-hub", "--transfers", "off"]

    private let service = SMAppService.loginItem(identifier: engineBundleIdentifier)
    private let loginItem = SMAppService.mainApp
    private let fileManager = FileManager.default

    private init() {}

    func activate(forceRestart: Bool = false) async throws {
        registerBundleLocations()
        let legacyURL = legacyPlistURL
        try ensureArgumentsFile(migrating: legacyURL)
        let legacyBackup = try await removeLegacyAgentIfPresent(at: legacyURL)
        do {
            try await ensureEngineRegistered(forceRestart: forceRestart || legacyBackup != nil)
        } catch {
            if let legacyBackup {
                try? await restoreLegacyAgent(legacyBackup)
            }
            throw error
        }
        try ensureLoginItemRegistered()
        UserDefaults.standard.removeObject(forKey: Self.lastErrorKey)
    }

    func openLoginItemsSettings() {
        SMAppService.openSystemSettingsLoginItems()
    }

    private func ensureEngineRegistered(forceRestart: Bool) async throws {
        let engineAppURL = Bundle.main.bundleURL
            .appendingPathComponent("Contents/Library/LoginItems", isDirectory: true)
            .appendingPathComponent("Tailboard Engine.app", isDirectory: true)
        let engineURL = engineAppURL
            .appendingPathComponent("Contents/MacOS/Tailboard Engine")
        guard fileManager.isExecutableFile(atPath: engineURL.path) else {
            throw EngineServiceError.embeddedEngineMissing
        }
        var registrationContent = try Data(contentsOf: engineURL)
        registrationContent.append(
            try Data(contentsOf: engineAppURL.appendingPathComponent("Contents/Info.plist"))
        )
        let engineHash = SHA256.hash(data: registrationContent)
            .map { String(format: "%02x", $0) }
            .joined()
        let storedHash = UserDefaults.standard.string(forKey: Self.registeredHashKey)

        switch service.status {
        case .requiresApproval:
            throw EngineServiceError.approvalRequired
        case .enabled:
            let isRunning = !NSRunningApplication.runningApplications(
                withBundleIdentifier: Self.engineBundleIdentifier
            ).isEmpty
            if forceRestart || storedHash != engineHash || !isRunning {
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

    /// Keep the SMAppService registration stable across application updates.
    /// Re-registering an existing background item during an in-place update is
    /// unreliable on macOS 27 betas, while Launch Services can safely restart
    /// the already-approved embedded login-item bundle at its stable URL.
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

        let configuration = NSWorkspace.OpenConfiguration()
        configuration.activates = false
        _ = try await NSWorkspace.shared.openApplication(
            at: url,
            configuration: configuration
        )
    }

    private func ensureLoginItemRegistered() throws {
        switch loginItem.status {
        case .notRegistered, .notFound:
            do {
                try loginItem.register()
            } catch {
                throw EngineServiceError.registrationFailed(error.localizedDescription)
            }
        case .enabled, .requiresApproval:
            // Respect an explicit user choice to disable Open at Login.
            break
        @unknown default:
            break
        }
    }

    private func unregister(_ appService: SMAppService) async throws {
        try await withCheckedThrowingContinuation { (continuation: CheckedContinuation<Void, Error>) in
            appService.unregister { error in
                if let error {
                    continuation.resume(throwing: error)
                } else {
                    continuation.resume()
                }
            }
        }
    }

    private func ensureArgumentsFile(migrating legacyURL: URL) throws {
        let existingData = try? Data(contentsOf: argumentsFileURL)
        let existing = existingData.flatMap {
            try? JSONSerialization.jsonObject(with: $0) as? [String]
        }

        let sourceArguments: [String]
        if let existing {
            sourceArguments = existing
        } else if let legacyArguments = try legacyProgramArguments(at: legacyURL), legacyArguments.count > 1 {
            sourceArguments = Array(legacyArguments.dropFirst())
        } else {
            sourceArguments = Self.defaultArguments
        }
        let arguments = Self.disablingLegacyTransfers(in: sourceArguments)
        if existing == arguments { return }

        try fileManager.createDirectory(
            at: argumentsFileURL.deletingLastPathComponent(),
            withIntermediateDirectories: true
        )
        let data = try JSONSerialization.data(withJSONObject: arguments, options: [.prettyPrinted, .sortedKeys])
        try data.write(to: argumentsFileURL, options: .atomic)
        try fileManager.setAttributes(
            [.posixPermissions: 0o600],
            ofItemAtPath: argumentsFileURL.path
        )
    }

    /// Taildrop owns file delivery. Rewrite prior app-managed arguments so an
    /// upgrade stops advertising or receiving Tailboard's legacy transfers.
    private static func disablingLegacyTransfers(in arguments: [String]) -> [String] {
        var result: [String] = []
        var index = 0
        var foundPolicy = false
        while index < arguments.count {
            let argument = arguments[index]
            if argument == "--transfers" {
                result.append(contentsOf: ["--transfers", "off"])
                foundPolicy = true
                index += min(2, arguments.count - index)
                continue
            }
            if argument.hasPrefix("--transfers=") {
                result.append("--transfers=off")
                foundPolicy = true
                index += 1
                continue
            }
            if argument == "--transfer-allow" {
                index += min(2, arguments.count - index)
                continue
            }
            if argument.hasPrefix("--transfer-allow=") {
                index += 1
                continue
            }
            result.append(argument)
            index += 1
        }
        if !foundPolicy {
            result.append(contentsOf: ["--transfers", "off"])
        }
        return result
    }

    private func legacyProgramArguments(at url: URL) throws -> [String]? {
        guard fileManager.fileExists(atPath: url.path) else { return nil }
        let data = try Data(contentsOf: url)
        let plist = try PropertyListSerialization.propertyList(from: data, options: [], format: nil)
        return (plist as? [String: Any])?["ProgramArguments"] as? [String]
    }

    private func removeLegacyAgentIfPresent(at url: URL) async throws -> LegacyAgentBackup? {
        guard fileManager.fileExists(atPath: url.path) else { return nil }
        let backup = LegacyAgentBackup(url: url, data: try Data(contentsOf: url))

        _ = try? runLaunchctl(["bootout", "gui/\(getuid())/\(Self.legacyServiceLabel)"])

        try fileManager.removeItem(at: url)
        UserDefaults.standard.removeObject(forKey: Self.registeredHashKey)
        try await Task.sleep(for: .milliseconds(500))
        return backup
    }

    private func restoreLegacyAgent(_ backup: LegacyAgentBackup) async throws {
        if service.status == .enabled {
            try? await unregister(service)
        }
        try fileManager.createDirectory(
            at: backup.url.deletingLastPathComponent(),
            withIntermediateDirectories: true
        )
        try backup.data.write(to: backup.url, options: .atomic)
        try fileManager.setAttributes(
            [.posixPermissions: 0o644],
            ofItemAtPath: backup.url.path
        )
        _ = try? runLaunchctl(["enable", "gui/\(getuid())/\(Self.legacyServiceLabel)"])
        try runLaunchctl(["bootstrap", "gui/\(getuid())", backup.url.path])
    }

    @discardableResult
    private func runLaunchctl(_ arguments: [String]) throws -> Int32 {
        let process = Process()
        process.executableURL = URL(fileURLWithPath: "/bin/launchctl")
        process.arguments = arguments
        try process.run()
        process.waitUntilExit()
        guard process.terminationStatus == 0 else {
            throw EngineServiceError.registrationFailed(
                "launchctl \(arguments.joined(separator: " ")) exited \(process.terminationStatus)"
            )
        }
        return process.terminationStatus
    }

    private var legacyPlistURL: URL {
        fileManager.homeDirectoryForCurrentUser
            .appendingPathComponent("Library/LaunchAgents", isDirectory: true)
            .appendingPathComponent(Self.legacyServicePlistName)
    }

    private var argumentsFileURL: URL {
        fileManager.urls(for: .applicationSupportDirectory, in: .userDomainMask)[0]
            .appendingPathComponent("tg-clipboard", isDirectory: true)
            .appendingPathComponent("engine-arguments.json")
    }
}
