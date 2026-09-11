import Foundation

/// Finds leftover files whose inferred ownership does not match any installed application.
public final class OrphanSearchEngine: @unchecked Sendable {
    public static let shared = OrphanSearchEngine()

    private let discovery: ResourceDiscoveryEngine
    private let denylist: SafetyDenylist
    private let fileManager: FileManager

    public init(
        discovery: ResourceDiscoveryEngine = .shared,
        denylist: SafetyDenylist = .shared,
        fileManager: FileManager = .default
    ) {
        self.discovery = discovery
        self.denylist = denylist
        self.fileManager = fileManager
    }

    public func findOrphans(
        installedApps: [AppIdentity],
        searchPaths: [DiscoveryPath]? = nil
    ) -> [OrphanedResource] {
        let installedIDs = Set(installedApps.map { $0.bundleIdentifier.lowercased() }.filter { !$0.isEmpty })
        let installedNames = Set(installedApps.map { $0.normalizedName }.filter { !$0.isEmpty })
        let vendorPrefixes = Set(installedApps.compactMap { $0.vendorPrefix?.lowercased() })

        let entries = discovery.enumerateAllShallowEntries(searchPaths: searchPaths)
        var orphans: [OrphanedResource] = []

        for entry in entries {
            if denylist.isDenied(path: entry.path) { continue }

            let name = (entry.path as NSString).lastPathComponent
            let lower = name.lowercased()

            // Prefer reverse-DNS shaped names (com.vendor.app...)
            let inferredID = inferBundleID(from: name)
            if let inferredID {
                let lowerID = inferredID.lowercased()
                if lowerID.hasPrefix("com.apple.") { continue }
                if installedIDs.contains(lowerID) { continue }

                // If any installed app uses this exact ID as prefix match for a longer container — skip
                if installedIDs.contains(where: { lowerID.hasPrefix($0 + ".") || $0.hasPrefix(lowerID + ".") }) {
                    // Still might be leftover of removed sibling — keep only if exact ID not installed
                    if installedIDs.contains(lowerID) { continue }
                }

                // Vendor still present → lower confidence (might be shared leftover of same vendor)
                let vendorStillPresent = vendorPrefixes.contains { lowerID.hasPrefix($0 + ".") || lowerID == $0 }
                let confidence: ConfidenceLevel = vendorStillPresent ? .low : .medium

                orphans.append(
                    OrphanedResource(
                        resource: entry,
                        inferredBundleID: inferredID,
                        inferredName: nil,
                        confidence: confidence,
                        isSelected: false
                    )
                )
                continue
            }

            // Saved state pattern: Something.savedState
            if entry.category == .savedState, lower.hasSuffix(".savedstate") {
                let base = String(lower.dropLast(".savedstate".count))
                if installedIDs.contains(base) { continue }
                if base.hasPrefix("com.apple.") { continue }
                orphans.append(
                    OrphanedResource(
                        resource: entry,
                        inferredBundleID: base.contains(".") ? base : nil,
                        inferredName: base,
                        confidence: .medium,
                        isSelected: false
                    )
                )
                continue
            }

            // Preferences: com.foo.bar.plist
            if entry.category == .preferences, lower.hasSuffix(".plist") {
                let base = String(name.dropLast(6)) // remove .plist preserving case for ID
                if let id = inferBundleID(from: base) {
                    let lowerID = id.lowercased()
                    if lowerID.hasPrefix("com.apple.") { continue }
                    if installedIDs.contains(lowerID) { continue }
                    orphans.append(
                        OrphanedResource(
                            resource: entry,
                            inferredBundleID: id,
                            inferredName: nil,
                            confidence: .high,
                            isSelected: false
                        )
                    )
                }
            }
        }

        // Deduplicate by path
        var seen = Set<String>()
        return orphans.filter { seen.insert($0.resource.path).inserted }
            .sorted {
                if $0.confidence != $1.confidence { return $0.confidence < $1.confidence }
                return $0.resource.path < $1.resource.path
            }
    }

    /// Heuristic: reverse-DNS token with at least two dots, or container folder names.
    public func inferBundleID(from name: String) -> String? {
        var candidate = name
        let suffixes = [".plist", ".savedState", ".savedstate", ".log", ".crash", ".ips"]
        for suffix in suffixes {
            if candidate.lowercased().hasSuffix(suffix.lowercased()) {
                candidate = String(candidate.dropLast(suffix.count))
                break
            }
        }
        // Group container: TEAMID.com.vendor.app
        if let range = candidate.range(of: #"^[A-Z0-9]{8,12}\.(.+)$"#, options: .regularExpression) {
            let rest = String(candidate[range]).split(separator: ".").dropFirst().joined(separator: ".")
            if rest.split(separator: ".").count >= 2 {
                return rest
            }
        }

        let parts = candidate.split(separator: ".")
        guard parts.count >= 3 else { return nil }
        // Must look like reverse DNS (letters in first component)
        guard parts[0].rangeOfCharacter(from: CharacterSet.letters) != nil else { return nil }
        return candidate
    }
}
