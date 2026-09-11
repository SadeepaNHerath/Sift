import Foundation
import UninstallerCore

/// Privileged helper daemon entry point. Talks to the main app over XPC.
final class HelperDelegate: NSObject, NSXPCListenerDelegate {
    func listener(_ listener: NSXPCListener, shouldAcceptNewConnection newConnection: NSXPCConnection) -> Bool {
        guard validateClient(connection: newConnection) else {
            return false
        }
        newConnection.exportedInterface = NSXPCInterface(with: UninstallerHelperProtocol.self)
        newConnection.exportedObject = HelperService()
        newConnection.resume()
        return true
    }

    /// Best-effort client validation. With ad-hoc signing, Team ID checks may be unavailable;
    /// production builds should tighten with SecCodeCopyGuestWithAttributes.
    private func validateClient(connection: NSXPCConnection) -> Bool {
        connection.processIdentifier > 0
    }
}

final class HelperService: NSObject, UninstallerHelperProtocol {
    private let denylist = SafetyDenylist.shared
    private let fileManager = FileManager.default

    func ping(withReply reply: @escaping (String) -> Void) {
        reply("pong")
    }

    func trashItems(at paths: [String], withReply reply: @escaping ([String: String]) -> Void) {
        var results: [String: String] = [:]
        for path in paths {
            if let reason = denylist.denyReason(for: path) {
                results[path] = reason
                continue
            }
            let standardized = (path as NSString).standardizingPath
            guard standardized.hasPrefix("/Library/") || standardized.hasPrefix("/usr/local/") else {
                results[path] = "Helper only trashes /Library or /usr/local paths"
                continue
            }
            let url = URL(fileURLWithPath: standardized)
            do {
                var resulting: NSURL?
                try fileManager.trashItem(at: url, resultingItemURL: &resulting)
                results[path] = ""
            } catch {
                results[path] = error.localizedDescription
            }
        }
        reply(results)
    }

    func bootoutLaunchd(label: String, domain: String, withReply reply: @escaping (String?) -> Void) {
        guard domain == "system" || domain.hasPrefix("gui/") else {
            reply("Invalid launchd domain")
            return
        }
        if label.lowercased().hasPrefix("com.apple.") {
            reply("Refusing to unload Apple system services")
            return
        }
        let process = Process()
        process.executableURL = URL(fileURLWithPath: "/bin/launchctl")
        process.arguments = ["bootout", "\(domain)/\(label)"]
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
let listener = NSXPCListener(machServiceName: HelperConstants.machServiceName)
listener.delegate = delegate
listener.resume()
RunLoop.current.run()
