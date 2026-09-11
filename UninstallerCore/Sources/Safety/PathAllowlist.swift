import Foundation

/// Strict allowlist for privileged (and shared) trash targets.
/// Prefer allowlisting known uninstall roots over denylist-only checks.
public enum PathAllowlist: Sendable {
    public static let allowedLibraryRoots: [String] = [
        "/Library/LaunchAgents",
        "/Library/LaunchDaemons",
        "/Library/Application Support",
        "/Library/Preferences",
        "/Library/Caches",
        "/Library/PrivilegedHelperTools"
    ]

    public static let blockedLibraryPrefixes: [String] = [
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

    /// Returns nil if the path is allowed for privileged trash; otherwise a reason string.
    public static func privilegedTrashDenialReason(for rawPath: String) -> String? {
        let url = URL(fileURLWithPath: rawPath)
        let resolved = url.resolvingSymlinksInPath().path
        let standardized = (resolved as NSString).standardizingPath

        if standardized != (rawPath as NSString).standardizingPath {
            // Symlink involved — only accept if resolved path is still allowlisted.
        }

        // Refuse if the leaf itself is a symlink (replaceable TOCTOU vector).
        if isSymlink(at: rawPath) {
            return "Refusing to trash symlink leaves"
        }

        if standardized == "/" || standardized.isEmpty {
            return "Refusing filesystem root"
        }

        // Never allow /usr/local or any /usr tree via helper.
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

        // Must be a leaf under an allowed root (not the root itself).
        for root in allowedLibraryRoots {
            if standardized == root {
                return "Refusing to trash shared folder root: \(root)"
            }
            if standardized.hasPrefix(root + "/") {
                let remainder = String(standardized.dropFirst(root.count + 1))
                // Must have at least one path component under the root.
                if remainder.isEmpty { return "Refusing empty leaf under \(root)" }
                // Prefer single-level leaves for Preferences/Caches/PrivilegedHelperTools/Launch*
                // Application Support may be a directory tree leaf (one top-level name).
                let components = remainder.split(separator: "/").map(String.init)
                guard let first = components.first, !first.isEmpty else {
                    return "Invalid path under \(root)"
                }
                if first.lowercased().hasPrefix("com.apple.") {
                    return "Apple system component"
                }
                // Re-compose allowed path: root + first component only for shallow roots;
                // for Application Support allow nested paths under that first leaf.
                if root.hasSuffix("Application Support") {
                    return nil
                }
                // LaunchAgents/Daemons/Preferences/Caches/PrivilegedHelperTools: single leaf only
                if components.count == 1 {
                    return nil
                }
                return "Only single-leaf items allowed under \(root)"
            }
        }

        return "Path not in privileged allowlist"
    }

    public static func isValidLaunchdLabel(_ label: String) -> Bool {
        // reverse-DNS-ish: letters/digits start, then letters digits . _ -
        let pattern = #"^[A-Za-z0-9][A-Za-z0-9._-]*$"#
        guard label.range(of: pattern, options: .regularExpression) != nil else { return false }
        if label.lowercased().hasPrefix("com.apple.") { return false }
        if label.contains("..") { return false }
        if label.contains("/") { return false }
        return true
    }

    /// Accepts only `system` or `gui/<uid>` where uid matches the peer (or current) uid when provided.
    public static func isValidLaunchdDomain(_ domain: String, peerUID: uid_t?) -> Bool {
        if domain == "system" { return true }
        guard domain.hasPrefix("gui/") else { return false }
        let rest = String(domain.dropFirst(4))
        guard let value = UInt32(rest) else { return false }
        if let peerUID, value != peerUID { return false }
        return true
    }

    public static func isSymlink(at path: String) -> Bool {
        var isDir: ObjCBool = false
        guard FileManager.default.fileExists(atPath: path, isDirectory: &isDir) else {
            // If missing, still check with lstat-style via URL resource values if possible
            let values = try? URL(fileURLWithPath: path).resourceValues(forKeys: [.isSymbolicLinkKey])
            return values?.isSymbolicLink == true
        }
        let values = try? URL(fileURLWithPath: path).resourceValues(forKeys: [.isSymbolicLinkKey])
        return values?.isSymbolicLink == true
    }

    /// Path-component boundary match for bundle IDs (not substring).
    public static func pathMatchesBundleID(_ path: String, bundleID: String) -> Bool {
        guard !bundleID.isEmpty else { return false }
        let needle = bundleID.lowercased()
        let name = (path as NSString).lastPathComponent.lowercased()
        // Exact filename or filename with known suffixes
        let suffixes = [".plist", ".savedstate", ".log", ".crash", ".ips"]
        if name == needle { return true }
        for suffix in suffixes {
            if name == needle + suffix { return true }
        }
        if name.hasPrefix(needle + ".") { return true }

        let parts = path.lowercased().split(separator: "/").map(String.init)
        return parts.contains { component in
            if component == needle { return true }
            for suffix in suffixes where component == needle + suffix { return true }
            return component.hasPrefix(needle + ".")
        }
    }

    public static func programPointsIntoApp(program: String, appPath: String) -> Bool {
        let app = (appPath as NSString).standardizingPath
        let prog = (program as NSString).standardizingPath
        if prog == app { return true }
        return prog.hasPrefix(app + "/")
    }
}
