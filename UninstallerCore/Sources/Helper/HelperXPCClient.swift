import Foundation
import ServiceManagement
import Security
import os.log

private let xpcLog = Logger(subsystem: "com.safeuninstaller.app", category: "HelperXPC")

/// Client that talks to the privileged UninstallerHelper over XPC and manages SMAppService registration.
public final class HelperXPCClient: NSObject, PrivilegedRemovalClient, @unchecked Sendable {
    public static let shared = HelperXPCClient()

    public static let defaultTimeout: TimeInterval = 15

    private var connection: NSXPCConnection?
    private let lock = NSLock()
    private let service = SMAppService.daemon(plistName: HelperConstants.helperPlistName)

    public var isAvailable: Bool {
        lock.lock()
        defer { lock.unlock() }
        if connection != nil { return true }
        return service.status == .enabled
    }

    /// True when the running app is signed with a real Team ID (not ad-hoc).
    public var isAppProperlySigned: Bool {
        CodeSigningInfo.currentTeamID() != nil
    }

    public var canRegisterHelper: Bool {
        isAppProperlySigned
    }

    public var registrationStatusDescription: String {
        if !isAppProperlySigned {
            return "Requires Developer ID signing"
        }
        switch service.status {
        case .enabled: return "Enabled"
        case .requiresApproval: return "Requires Approval in System Settings → Login Items"
        case .notFound: return "Not Found (helper not embedded or not signed)"
        case .notRegistered: return "Not Registered"
        @unknown default: return "Unknown"
        }
    }

    public func register() throws {
        guard canRegisterHelper else {
            throw NSError(
                domain: "SafeUninstaller",
                code: 10,
                userInfo: [NSLocalizedDescriptionKey: "Privileged helper registration requires Developer ID / Apple Development signing with a Team ID."]
            )
        }
        try service.register()
    }

    public func unregister() throws {
        try service.unregister()
        lock.lock()
        connection?.invalidate()
        connection = nil
        lock.unlock()
    }

    public func ensureConnection() throws -> NSXPCConnection {
        lock.lock()
        defer { lock.unlock() }
        if let connection { return connection }
        let conn = NSXPCConnection(machServiceName: HelperConstants.machServiceName, options: .privileged)
        conn.remoteObjectInterface = NSXPCInterface(with: UninstallerHelperProtocol.self)
        conn.invalidationHandler = { [weak self] in
            xpcLog.error("Helper XPC connection invalidated")
            self?.lock.lock()
            self?.connection = nil
            self?.lock.unlock()
        }
        conn.interruptionHandler = { [weak self] in
            xpcLog.error("Helper XPC connection interrupted")
            self?.lock.lock()
            self?.connection = nil
            self?.lock.unlock()
        }
        conn.resume()
        connection = conn
        return conn
    }

    private func proxy(errorHandler: @escaping (Error) -> Void) throws -> UninstallerHelperProtocol {
        let conn = try ensureConnection()
        guard let proxy = conn.remoteObjectProxyWithErrorHandler(errorHandler) as? UninstallerHelperProtocol else {
            throw NSError(
                domain: "SafeUninstaller",
                code: 1,
                userInfo: [NSLocalizedDescriptionKey: "Unable to create helper proxy"]
            )
        }
        return proxy
    }

    public func trashItems(at paths: [String]) async throws -> [String: String] {
        try await withTimeout(Self.defaultTimeout) {
            try await withCheckedThrowingContinuation { (continuation: CheckedContinuation<[String: String], Error>) in
                let box = ResumeBox(continuation)
                do {
                    let proxy = try self.proxy { error in
                        box.resume(throwing: error)
                    }
                    proxy.trashItems(at: paths) { result in
                        box.resume(returning: result)
                    }
                } catch {
                    box.resume(throwing: error)
                }
            }
        }
    }

    public func bootoutLaunchd(label: String, domain: String) async throws {
        try await withTimeout(Self.defaultTimeout) {
            try await withCheckedThrowingContinuation { (continuation: CheckedContinuation<Void, Error>) in
                let box = ResumeBox(continuation)
                do {
                    let proxy = try self.proxy { error in
                        box.resume(throwing: error)
                    }
                    proxy.bootoutLaunchd(label: label, domain: domain) { errorString in
                        if let errorString, !errorString.isEmpty {
                            box.resume(
                                throwing: NSError(
                                    domain: "SafeUninstaller",
                                    code: 2,
                                    userInfo: [NSLocalizedDescriptionKey: errorString]
                                )
                            )
                        } else {
                            box.resume(returning: ())
                        }
                    }
                } catch {
                    box.resume(throwing: error)
                }
            }
        }
    }

    public func ping() async -> String? {
        do {
            return try await withTimeout(5) {
                try await withCheckedThrowingContinuation { (continuation: CheckedContinuation<String, Error>) in
                    let box = ResumeBox(continuation)
                    do {
                        let proxy = try self.proxy { error in
                            box.resume(throwing: error)
                        }
                        proxy.ping { reply in
                            box.resume(returning: reply)
                        }
                    } catch {
                        box.resume(throwing: error)
                    }
                }
            }
        } catch {
            xpcLog.error("ping failed: \(error.localizedDescription)")
            return nil
        }
    }

    private func withTimeout<T: Sendable>(
        _ seconds: TimeInterval,
        operation: @escaping @Sendable () async throws -> T
    ) async throws -> T {
        try await withThrowingTaskGroup(of: T.self) { group in
            group.addTask { try await operation() }
            group.addTask {
                try await Task.sleep(nanoseconds: UInt64(seconds * 1_000_000_000))
                throw NSError(
                    domain: "SafeUninstaller",
                    code: 3,
                    userInfo: [NSLocalizedDescriptionKey: "Helper XPC timed out after \(Int(seconds))s"]
                )
            }
            let result = try await group.next()!
            group.cancelAll()
            return result
        }
    }
}

/// Ensures a checked continuation is resumed at most once.
private final class ResumeBox<T>: @unchecked Sendable {
    private let lock = NSLock()
    private var continuation: CheckedContinuation<T, Error>?

    init(_ continuation: CheckedContinuation<T, Error>) {
        self.continuation = continuation
    }

    func resume(returning value: T) {
        lock.lock()
        let cont = continuation
        continuation = nil
        lock.unlock()
        cont?.resume(returning: value)
    }

    func resume(throwing error: Error) {
        lock.lock()
        let cont = continuation
        continuation = nil
        lock.unlock()
        cont?.resume(throwing: error)
    }
}

public enum CodeSigningInfo {
    public static func currentTeamID() -> String? {
        var code: SecCode?
        guard SecCodeCopySelf([], &code) == errSecSuccess, let code else { return nil }
        var staticCode: SecStaticCode?
        guard SecCodeCopyStaticCode(code, [], &staticCode) == errSecSuccess, let staticCode else { return nil }
        var info: CFDictionary?
        guard SecCodeCopySigningInformation(staticCode, SecCSFlags(rawValue: kSecCSSigningInformation), &info) == errSecSuccess,
              let dict = info as? [String: Any] else { return nil }
        return dict[kSecCodeInfoTeamIdentifier as String] as? String
    }
}
