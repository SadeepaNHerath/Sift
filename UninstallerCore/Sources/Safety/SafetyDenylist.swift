import Foundation

/// Hard denylist of paths and patterns that must never be selected for removal.
public final class SafetyDenylist: @unchecked Sendable {
    public static let shared = SafetyDenylist()

    private let protectedPrefixes: [String]
    private let protectedBundlePrefixes: [String]
    private let sensitiveHomeRelativePrefixes: [String]

    public init(
        protectedPrefixes: [String]? = nil,
        protectedBundlePrefixes: [String]? = nil,
        sensitiveHomeRelativePrefixes: [String]? = nil
    ) {
        self.protectedPrefixes = protectedPrefixes ?? [
            "/System",
            "/usr/bin",
            "/usr/sbin",
            "/usr/lib",
            "/usr/libexec",
            "/bin",
            "/sbin",
            "/private/var/db",
            "/Library/Apple",
            "/Library/OSAnalytics",
            "/Library/SystemExtensions",
            "/Library/Keychains",
            "/Library/Extensions",
            "/Library/Frameworks",
            "/Library/Security",
            "/Library/Managed Preferences",
            "/Library/Preferences/SystemConfiguration",
            "/Library/Filesystems",
            "/Library/Updates",
            "/Library/User Template",
            "/Library/WebServer",
            "/usr/local"
        ]
        self.protectedBundlePrefixes = protectedBundlePrefixes ?? [
            "com.apple.",
            "com.apple.security.",
            "com.apple.loginwindow"
        ]
        self.sensitiveHomeRelativePrefixes = sensitiveHomeRelativePrefixes ?? [
            "Library/Keychains",
            "Library/Mail",
            "Library/Messages",
            "Library/Safari",
            "Library/Accounts",
            "Library/Cookies",
            "Library/Calendars",
            "Library/Reminders",
            "Library/IdentityServices",
            "Library/Application Support/AddressBook",
            ".ssh",
            ".gnupg"
        ]
    }

    public func denyReason(for path: String, bundleHint: String? = nil) -> String? {
        // Resolve symlinks before evaluation.
        let resolved = URL(fileURLWithPath: path).resolvingSymlinksInPath().path
        let standardized = (resolved as NSString).standardizingPath

        if PathAllowlist.isSymlink(at: path) {
            return "Refusing symlink leaves"
        }

        if standardized == "/" || standardized.isEmpty {
            return "Refusing to operate on filesystem root"
        }

        for prefix in protectedPrefixes {
            if standardized == prefix || standardized.hasPrefix(prefix + "/") {
                return "SIP / system-protected path: \(prefix)"
            }
        }

        // Never allow deleting entire Library folders.
        let home = FileManager.default.homeDirectoryForCurrentUser.path
        let homeStandard = (home as NSString).standardizingPath

        if standardized == homeStandard {
            return "Refusing to delete home directory"
        }

        for relative in sensitiveHomeRelativePrefixes {
            let prefix = (homeStandard as NSString).appendingPathComponent(relative)
            if standardized == prefix || standardized.hasPrefix(prefix + "/") {
                return "Sensitive user data: \(relative)"
            }
        }

        let dangerousExact = [
            home + "/Library",
            home + "/Library/Preferences",
            home + "/Library/Application Support",
            home + "/Library/Caches",
            home + "/Library/Containers",
            home + "/Library/Group Containers",
            "/Library",
            "/Library/Preferences",
            "/Library/Application Support",
            "/Library/Caches",
            "/Library/LaunchAgents",
            "/Library/LaunchDaemons",
            "/Library/PrivilegedHelperTools",
            "/Applications"
        ]
        if dangerousExact.contains(standardized) {
            return "Refusing to delete shared system/user folder"
        }

        let name = (standardized as NSString).lastPathComponent.lowercased()
        if name.hasPrefix("com.apple.") || name.contains(".com.apple.") {
            return "Apple system component"
        }

        if let hint = bundleHint?.lowercased() {
            for prefix in protectedBundlePrefixes where hint.hasPrefix(prefix) {
                return "Apple / system bundle identifier"
            }
        }

        // Block all of /usr including /usr/local for safety in the main app path as well.
        if standardized.hasPrefix("/usr/") || standardized == "/usr" {
            return "System unix path under /usr"
        }

        return nil
    }

    public func isDenied(path: String, bundleHint: String? = nil) -> Bool {
        denyReason(for: path, bundleHint: bundleHint) != nil
    }

    /// Final gate immediately before trash — re-check denylist and live symlink status.
    public func preTrashDenialReason(for path: String) -> String? {
        if PathAllowlist.isSymlink(at: path) {
            return "Refusing symlink leaves"
        }
        return denyReason(for: path)
    }
}
