import Foundation
import Security

/// Scans standard macOS application directories and extracts bundle + signing identity.
public final class AppInventoryScanner: @unchecked Sendable {
    public static let shared = AppInventoryScanner()

    private let fileManager: FileManager
    private let searchRoots: [URL]

    public init(fileManager: FileManager = .default, searchRoots: [URL]? = nil) {
        self.fileManager = fileManager
        if let searchRoots {
            self.searchRoots = searchRoots
        } else {
            let homeApps = fileManager.homeDirectoryForCurrentUser
                .appendingPathComponent("Applications")
            self.searchRoots = [
                URL(fileURLWithPath: "/Applications"),
                URL(fileURLWithPath: "/Applications/Utilities"),
                homeApps
            ]
        }
    }

    public func scanInstalledApps() -> [AppIdentity] {
        var apps: [AppIdentity] = []
        var seen = Set<String>()

        for root in searchRoots {
            guard fileManager.fileExists(atPath: root.path) else { continue }
            let enumerator = fileManager.enumerator(
                at: root,
                includingPropertiesForKeys: [.isApplicationKey, .isDirectoryKey],
                options: [.skipsHiddenFiles, .skipsPackageDescendants]
            )
            while let url = enumerator?.nextObject() as? URL {
                guard url.pathExtension.lowercased() == "app" else { continue }
                if let identity = identify(at: url), seen.insert(identity.path).inserted {
                    apps.append(identity)
                }
                enumerator?.skipDescendants()
            }
        }

        return apps.sorted {
            $0.name.localizedCaseInsensitiveCompare($1.name) == .orderedAscending
        }
    }

    public func identify(at url: URL) -> AppIdentity? {
        let path = url.path
        guard fileManager.fileExists(atPath: path) else { return nil }

        let infoURL = url.appendingPathComponent("Contents/Info.plist")
        let plist = NSDictionary(contentsOf: infoURL) as? [String: Any] ?? [:]

        let bundleID = (plist["CFBundleIdentifier"] as? String) ?? ""
        let name = (plist["CFBundleDisplayName"] as? String)
            ?? (plist["CFBundleName"] as? String)
            ?? url.deletingPathExtension().lastPathComponent
        let version = (plist["CFBundleShortVersionString"] as? String)
            ?? (plist["CFBundleVersion"] as? String)
            ?? ""
        let executable = (plist["CFBundleExecutable"] as? String) ?? ""

        let signing = extractSigningInfo(at: url)

        return AppIdentity(
            path: path,
            bundleIdentifier: bundleID,
            name: name,
            version: version,
            executableName: executable,
            teamID: signing.teamID,
            signingAuthority: signing.authority,
            isAppleSigned: signing.isApple
        )
    }

    public func identifyFromDroppedApp(url: URL) -> AppIdentity? {
        identify(at: url)
    }

    private struct SigningInfo {
        let teamID: String?
        let authority: String?
        let isApple: Bool
    }

    private func extractSigningInfo(at url: URL) -> SigningInfo {
        var staticCode: SecStaticCode?
        let status = SecStaticCodeCreateWithPath(url as CFURL, [], &staticCode)
        guard status == errSecSuccess, let staticCode else {
            return SigningInfo(teamID: nil, authority: nil, isApple: false)
        }

        var info: CFDictionary?
        let infoStatus = SecCodeCopySigningInformation(
            staticCode,
            SecCSFlags(rawValue: kSecCSSigningInformation),
            &info
        )
        guard infoStatus == errSecSuccess, let dict = info as? [String: Any] else {
            return SigningInfo(teamID: nil, authority: nil, isApple: false)
        }

        let teamID = dict[kSecCodeInfoTeamIdentifier as String] as? String
        let authorities = dict[kSecCodeInfoCertificates as String] as? [SecCertificate]
        var authorityName: String?
        if let first = authorities?.first {
            authorityName = SecCertificateCopySubjectSummary(first) as String?
        }
        let isApple = (authorityName?.contains("Apple") == true)
            || (teamID == nil && bundleLooksLikeSystemApp(url: url))

        return SigningInfo(teamID: teamID, authority: authorityName, isApple: isApple)
    }

    private func bundleLooksLikeSystemApp(url: URL) -> Bool {
        url.path.hasPrefix("/System/Applications") || url.path.hasPrefix("/System/Library")
    }
}
