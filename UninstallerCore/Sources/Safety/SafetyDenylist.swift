import Foundation

/// Hard denylist of paths and patterns that must never be selected for removal.
public final class SafetyDenylist: @unchecked Sendable {
    public static let shared = SafetyDenylist()

    private let protectedPrefixes: [String]
    private let protectedBundlePrefixes: [String]

    public init(
        protectedPrefixes: [String]? = nil,
        protectedBundlePrefixes: [String]? = nil
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
            "/Library/SystemExtensions"
        ]
        self.protectedBundlePrefixes = protectedBundlePrefixes ?? [
            "com.apple.",
            "com.apple.security.",
            "com.apple.loginwindow"
        ]
    }

    public func denyReason(for path: String, bundleHint: String? = nil) -> String? {
        let standardized = (path as NSString).standardizingPath

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
            "/Library/LaunchAgents",
            "/Library/LaunchDaemons",
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

        // Allow /usr/local explicitly (not SIP).
        if standardized.hasPrefix("/usr/") && !standardized.hasPrefix("/usr/local") {
            return "System unix path under /usr"
        }

        return nil
    }

    public func isDenied(path: String, bundleHint: String? = nil) -> Bool {
        denyReason(for: path, bundleHint: bundleHint) != nil
    }
}
