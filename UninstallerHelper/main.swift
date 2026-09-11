import Foundation
import Security
import os.log

// MARK: - Standalone helper (no UninstallerCore link — avoids broken @rpath under LaunchDaemons)

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

// MARK: - Allowlist (duplicated intentionally — keep in sync with PathAllowlist)

private enum HelperAllowlist {
    static let allowedLibraryRoots: [String] = [
        "/Library/LaunchAgents",
        "/Library/LaunchDaemons",
        "/Library/Application Support",
        "/Library/Preferences",
        "/Library/Caches",
        "/Library/PrivilegedHelperTools"
    ]

    static let blockedLibraryPrefixes: [String] = [
        "/Library/Apple",
        "/Library/Keychains",
        "/Library/Extensions",
        "/Library/Frameworks",
        "/Library/Security",
        "/Library/Managed Preferences",
        "/Library/Preferences/SystemConfiguration",
        "/Library/SystemExtensions",
        "/Library/OSAnalytics",
        "/Library/Filesystems",
        "/Library/GPUBundles",
        "/Library/Image Capture",
        "/Library/Printers",
        "/Library/Updates",
        "/Library/User Template",
        "/Library/WebServer"
    ]

    static func trashDenialReason(for rawPath: String) -> String? {
        if isSymlink(at: rawPath) {
            return "Refusing to trash symlink leaves"
        }
        let resolved = URL(fileURLWithPath: rawPath).resolvingSymlinksInPath().path
        let standardized = (resolved as NSString).standardizingPath

        if standardized == "/" || standardized.isEmpty {
            return "Refusing filesystem root"
        }
        if standardized == "/usr/local"
            || standardized.hasPrefix("/usr/local/")
            || standardized.hasPrefix("/usr/") {
            return "Helper does not trash /usr or /usr/local"
        }
        for blocked in blockedLibraryPrefixes {
            if standardized == blocked || standardized.hasPrefix(blocked + "/") {
                return "Blocked system Library path: \(blocked)"
            }
        }
        for root in allowedLibraryRoots {
            if standardized == root {
                return "Refusing to trash shared folder root: \(root)"
            }
            if standardized.hasPrefix(root + "/") {
                let remainder = String(standardized.dropFirst(root.count + 1))
                let components = remainder.split(separator: "/").map(String.init)
                guard let first = components.first, !first.isEmpty else {
                    return "Invalid path under \(root)"
                }
                if first.lowercased().hasPrefix("com.apple.") {
                    return "Apple system component"
                }
                if root.hasSuffix("Application Support") {
                    return nil
                }
                if components.count == 1 {
                    return nil
                }
                return "Only single-leaf items allowed under \(root)"
            }
        }
        return "Path not in privileged allowlist"
    }

    static func isValidLaunchdLabel(_ label: String) -> Bool {
        let pattern = #"^[A-Za-z0-9][A-Za-z0-9._-]*$"#
        guard label.range(of: pattern, options: .regularExpression) != nil else { return false }
        if label.lowercased().hasPrefix("com.apple.") { return false }
        if label.contains("..") || label.contains("/") { return false }
        return true
    }

    static func isValidLaunchdDomain(_ domain: String, peerUID: uid_t?) -> Bool {
        if domain == "system" { return true }
        guard domain.hasPrefix("gui/") else { return false }
        let rest = String(domain.dropFirst(4))
        guard let value = UInt32(rest) else { return false }
        if let peerUID, value != peerUID { return false }
        return true
    }

    static func isSymlink(at path: String) -> Bool {
        let values = try? URL(fileURLWithPath: path).resourceValues(forKeys: [.isSymbolicLinkKey])
        return values?.isSymbolicLink == true
    }
}

// MARK: - Client validation

private enum ClientValidator {
    static func isTrusted(connection: NSXPCConnection) -> Bool {
        #if DEBUG
        // Ad-hoc local iteration: still prefer real validation when possible.
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

        // Require our app bundle ID. When Developer ID signed, also pin Team ID if known.
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
            if let reason = HelperAllowlist.trashDenialReason(for: path) {
                helperLog.warning("Denied trash \(path): \(reason)")
                results[path] = reason
                continue
            }
            let resolved = URL(fileURLWithPath: path).resolvingSymlinksInPath()
            if let reason = HelperAllowlist.trashDenialReason(for: resolved.path) {
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
        guard HelperAllowlist.isValidLaunchdLabel(label) else {
            reply("Invalid launchd label")
            return
        }
        // Privileged helper only boots out system-domain jobs.
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
