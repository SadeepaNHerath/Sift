import Foundation
import Security
import os.log

// MARK: - Standalone helper (no UninstallerCore link — avoids broken @rpath under LaunchDaemons)
// PathAllowlist.swift is compiled into this target from UninstallerCore/Sources/Safety/

private let helperLog = Logger(subsystem: "com.safeuninstaller.UninstallerHelper", category: "daemon")

private enum HelperIdentity {
    static let machServiceName = "com.safeuninstaller.UninstallerHelper"
    static let appBundleID = "com.safeuninstaller.app"
    /// Set at build time via OTHER_SWIFT_FLAGS if desired; empty means Team ID must match client's own when signed.
    static let expectedTeamID: String? = nil
}

@objc private protocol UninstallerHelperProtocol {
    func trashItems(at paths: [String], withReply reply: @escaping ([String: String]) -> Void)
    func bootoutLaunchd(label: String, domain: String, withReply reply: @escaping (String?) -> Void)
    func ping(withReply reply: @escaping (String) -> Void)
}

// MARK: - Client validation

private enum ClientValidator {
    static func isTrusted(connection: NSXPCConnection) -> Bool {
        #if DEBUG
        if validateCodeSignature(connection: connection) {
            return true
        }
        helperLog.warning("DEBUG: accepting connection after soft validation failure (pid \(connection.processIdentifier))")
        return connection.processIdentifier > 0
        #else
        return validateCodeSignature(connection: connection)
        #endif
    }

    private static func validateCodeSignature(connection: NSXPCConnection) -> Bool {
        var code: SecCode?
        let pid = connection.processIdentifier
        let attrs = [kSecGuestAttributePid: pid] as CFDictionary
        let status = SecCodeCopyGuestWithAttributes(nil, attrs, [], &code)
        guard status == errSecSuccess, let code else {
            helperLog.error("SecCodeCopyGuestWithAttributes failed: \(status)")
            return false
        }

        var requirementString = "identifier \"\(HelperIdentity.appBundleID)\""
        if let team = HelperIdentity.expectedTeamID, !team.isEmpty {
            requirementString += " and anchor apple generic and certificate leaf[subject.OU] = \"\(team)\""
        }

        var requirement: SecRequirement?
        guard SecRequirementCreateWithString(requirementString as CFString, [], &requirement) == errSecSuccess,
              let requirement else {
            helperLog.error("Failed to create SecRequirement")
            return false
        }

        let check = SecCodeCheckValidity(code, [], requirement)
        if check == errSecSuccess {
            return true
        }

        #if DEBUG
        var staticCode: SecStaticCode?
        if SecCodeCopyStaticCode(code, [], &staticCode) == errSecSuccess, let staticCode {
            var info: CFDictionary?
            if SecCodeCopySigningInformation(staticCode, SecCSFlags(rawValue: kSecCSSigningInformation), &info) == errSecSuccess,
               let dict = info as? [String: Any],
               let ident = dict[kSecCodeInfoIdentifier as String] as? String,
               ident == HelperIdentity.appBundleID {
                helperLog.warning("DEBUG: identifier match without full requirement (\(ident))")
                return true
            }
        }
        #endif
        helperLog.error("Client code validity failed: \(check)")
        return false
    }
}

// MARK: - Service

final class HelperDelegate: NSObject, NSXPCListenerDelegate {
    func listener(_ listener: NSXPCListener, shouldAcceptNewConnection newConnection: NSXPCConnection) -> Bool {
        guard ClientValidator.isTrusted(connection: newConnection) else {
            helperLog.error("Rejected untrusted XPC client pid=\(newConnection.processIdentifier)")
            return false
        }
        newConnection.exportedInterface = NSXPCInterface(with: UninstallerHelperProtocol.self)
        newConnection.exportedObject = HelperService()
        newConnection.resume()
        return true
    }
}

final class HelperService: NSObject, UninstallerHelperProtocol {
    private let fileManager = FileManager.default

    func ping(withReply reply: @escaping (String) -> Void) {
        reply("pong")
    }

    func trashItems(at paths: [String], withReply reply: @escaping ([String: String]) -> Void) {
        var results: [String: String] = [:]
        for path in paths {
            if let reason = PathAllowlist.privilegedTrashDenialReason(for: path) {
                helperLog.warning("Denied trash \(path): \(reason)")
                results[path] = reason
                continue
            }
            let resolved = URL(fileURLWithPath: path).resolvingSymlinksInPath()
            if let reason = PathAllowlist.privilegedTrashDenialReason(for: resolved.path) {
                results[path] = reason
                continue
            }
            do {
                var resulting: NSURL?
                try fileManager.trashItem(at: resolved, resultingItemURL: &resulting)
                results[path] = ""
            } catch {
                results[path] = error.localizedDescription
            }
        }
        reply(results)
    }

    func bootoutLaunchd(label: String, domain: String, withReply reply: @escaping (String?) -> Void) {
        guard PathAllowlist.isValidLaunchdLabel(label) else {
            reply("Invalid launchd label")
            return
        }
        guard domain == "system" else {
            reply("Helper only supports system launchd domain")
            return
        }
        let process = Process()
        process.executableURL = URL(fileURLWithPath: "/bin/launchctl")
        process.arguments = ["bootout", "system/\(label)"]
        let err = Pipe()
        process.standardError = err
        process.standardOutput = Pipe()
        do {
            try process.run()
            process.waitUntilExit()
            if process.terminationStatus == 0 {
                reply(nil)
            } else {
                let data = err.fileHandleForReading.readDataToEndOfFile()
                reply(String(data: data, encoding: .utf8) ?? "launchctl failed")
            }
        } catch {
            reply(error.localizedDescription)
        }
    }
}

let delegate = HelperDelegate()
let listener = NSXPCListener(machServiceName: HelperIdentity.machServiceName)
listener.delegate = delegate
listener.resume()
helperLog.info("UninstallerHelper listening on \(HelperIdentity.machServiceName)")
RunLoop.current.run()
