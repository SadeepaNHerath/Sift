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
    public var vendorPrefix: String? {
        let parts = bundleIdentifier.split(separator: ".")
        guard parts.count >= 2 else { return nil }
        return parts.prefix(parts.count - 1).joined(separator: ".")
    }
}
