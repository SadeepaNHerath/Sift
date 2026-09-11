import Foundation
import ServiceManagement

/// Client that talks to the privileged UninstallerHelper over XPC and manages SMAppService registration.
public final class HelperXPCClient: NSObject, PrivilegedRemovalClient, @unchecked Sendable {
    public static let shared = HelperXPCClient()

    private var connection: NSXPCConnection?
    private let lock = NSLock()
    private let service = SMAppService.daemon(plistName: HelperConstants.helperPlistName)

    public var isAvailable: Bool {
        lock.lock()
        defer { lock.unlock() }
        if connection != nil { return true }
        return service.status == .enabled
    }

    public var registrationStatusDescription: String {
        switch service.status {
        case .enabled: return "Enabled"
        case .requiresApproval: return "Requires Approval in System Settings → Login Items"
        case .notFound: return "Not Found (helper not embedded or not signed)"
        case .notRegistered: return "Not Registered"
        @unknown default: return "Unknown"
        }
    }

    public func register() throws {
        try service.register()
    }

    public func unregister() throws {
        try service.unregister()
        lock.lock()
        connection?.invalidate()
        connection = nil
        lock.unlock()
    }

    public func ensureConnection() -> NSXPCConnection {
        lock.lock()
        defer { lock.unlock() }
        if let connection { return connection }
        let conn = NSXPCConnection(machServiceName: HelperConstants.machServiceName, options: .privileged)
        conn.remoteObjectInterface = NSXPCInterface(with: UninstallerHelperProtocol.self)
        conn.invalidationHandler = { [weak self] in
            self?.lock.lock()
            self?.connection = nil
            self?.lock.unlock()
        }
        conn.interruptionHandler = { [weak self] in
            self?.lock.lock()
            self?.connection = nil
            self?.lock.unlock()
        }
        conn.resume()
        connection = conn
        return conn
    }

    private func proxy() throws -> UninstallerHelperProtocol {
        let conn = ensureConnection()
        guard let proxy = conn.remoteObjectProxyWithErrorHandler({ _ in }) as? UninstallerHelperProtocol else {
            throw NSError(
                domain: "SafeUninstaller",
                code: 1,
                userInfo: [NSLocalizedDescriptionKey: "Unable to create helper proxy"]
            )
        }
        return proxy
    }

    public func trashItems(at paths: [String]) async throws -> [String: String] {
        let proxy = try proxy()
        return try await withCheckedThrowingContinuation { continuation in
            proxy.trashItems(at: paths) { result in
                continuation.resume(returning: result)
            }
        }
    }

    public func bootoutLaunchd(label: String, domain: String) async throws {
        let proxy = try proxy()
        try await withCheckedThrowingContinuation { (continuation: CheckedContinuation<Void, Error>) in
            proxy.bootoutLaunchd(label: label, domain: domain) { errorString in
                if let errorString, !errorString.isEmpty {
                    continuation.resume(
                        throwing: NSError(
                            domain: "SafeUninstaller",
                            code: 2,
                            userInfo: [NSLocalizedDescriptionKey: errorString]
                        )
                    )
                } else {
                    continuation.resume()
                }
            }
        }
    }

    public func ping() async -> String? {
        guard let proxy = try? proxy() else { return nil }
        return await withCheckedContinuation { continuation in
            proxy.ping { reply in
                continuation.resume(returning: reply)
            }
        }
    }
}
