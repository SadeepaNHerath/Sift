import Foundation

/// Identity of a macOS application bundle used for resource attribution.
public struct AppIdentity: Identifiable, Hashable, Codable, Sendable {
    public var id: String { bundleIdentifier.isEmpty ? path : bundleIdentifier }

    public let path: String
    public let bundleIdentifier: String
    public let name: String
    public let version: String
    public let executableName: String
    public let teamID: String?
    public let signingAuthority: String?
    public let isAppleSigned: Bool

    public init(
        path: String,
        bundleIdentifier: String,
        name: String,
        version: String = "",
        executableName: String = "",
        teamID: String? = nil,
        signingAuthority: String? = nil,
        isAppleSigned: Bool = false
    ) {
        self.path = path
        self.bundleIdentifier = bundleIdentifier
        self.name = name
        self.version = version
        self.executableName = executableName
        self.teamID = teamID
        self.signingAuthority = signingAuthority
        self.isAppleSigned = isAppleSigned
    }

    /// Normalized name for fuzzy path matching (lowercase, no spaces/punctuation).
    public var normalizedName: String {
        name.lowercased()
            .replacingOccurrences(of: " ", with: "")
            .replacingOccurrences(of: "-", with: "")
            .replacingOccurrences(of: "_", with: "")
            .replacingOccurrences(of: ".", with: "")
    }

    /// Reverse-DNS vendor prefix, e.g. "com.vendor" from "com.vendor.app".
    /// Rejects too-short prefixes like bare "com" / "net" / "org".
    public var vendorPrefix: String? {
        let parts = bundleIdentifier.split(separator: ".").map(String.init)
        guard parts.count >= 3 else { return nil }
        let prefix = parts.dropLast().joined(separator: ".")
        let blocked = Set(["com", "net", "org", "edu", "gov", "io", "co", "app"])
        if blocked.contains(prefix.lowercased()) { return nil }
        // Also require at least two labels in the vendor prefix (e.g. com.vendor)
        if prefix.split(separator: ".").count < 2 { return nil }
        return prefix
    }
}
