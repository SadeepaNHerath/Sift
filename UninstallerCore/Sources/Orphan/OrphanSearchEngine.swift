import Foundation

/// Finds leftover files whose inferred ownership does not match any currently installed application.
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
        let installedNames = Set(installedApps.map { $0.normalizedName }.filter { $0.count >= 4 })
        let vendorPrefixes = Set(installedApps.compactMap { $0.vendorPrefix?.lowercased() })

        let entries = discovery.enumerateAllShallowEntries(searchPaths: searchPaths)
        var orphans: [OrphanedResource] = []

        for entry in entries {
            if denylist.isDenied(path: entry.path) { continue }

            let name = (entry.path as NSString).lastPathComponent
            let lower = name.lowercased()

            if let inferredID = inferBundleID(from: name) {
                let lowerID = inferredID.lowercased()
                if lowerID.hasPrefix("com.apple.") { continue }
                if installedIDs.contains(lowerID) { continue }

                // Skip parent / child / sibling of any installed bundle ID
                if isRelated(to: lowerID, installedIDs: installedIDs) {
                    continue
                }

                let vendorStillPresent = vendorPrefixes.contains { lowerID.hasPrefix($0 + ".") || lowerID == $0 }
                let confidence: ConfidenceLevel = vendorStillPresent ? .low : .low

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

            if entry.category == .savedState, lower.hasSuffix(".savedstate") {
                let base = String(lower.dropLast(".savedstate".count))
                if installedIDs.contains(base) { continue }
                if isRelated(to: base, installedIDs: installedIDs) { continue }
                if base.hasPrefix("com.apple.") { continue }
                orphans.append(
                    OrphanedResource(
                        resource: entry,
                        inferredBundleID: base.contains(".") ? base : nil,
                        inferredName: base,
                        confidence: .low,
                        isSelected: false
                    )
                )
                continue
            }

            if entry.category == .preferences, lower.hasSuffix(".plist") {
                let base = String(name.dropLast(6))
                if let id = inferBundleID(from: base) {
                    let lowerID = id.lowercased()
                    if lowerID.hasPrefix("com.apple.") { continue }
                    if installedIDs.contains(lowerID) { continue }
                    if isRelated(to: lowerID, installedIDs: installedIDs) { continue }
                    orphans.append(
                        OrphanedResource(
                            resource: entry,
                            inferredBundleID: id,
                            inferredName: nil,
                            confidence: .medium,
                            isSelected: false
                        )
                    )
                }
                continue
            }

            // Name-only leftovers: only if normalized name matches no installed app
            let normalizedLeaf = lower
                .replacingOccurrences(of: " ", with: "")
                .replacingOccurrences(of: "-", with: "")
                .replacingOccurrences(of: "_", with: "")
                .replacingOccurrences(of: ".plist", with: "")
            if normalizedLeaf.count >= 5, installedNames.contains(normalizedLeaf) {
                continue
            }
        }

        var seen = Set<String>()
        return orphans.filter { seen.insert($0.resource.path).inserted }
            .sorted {
                if $0.confidence != $1.confidence { return $0.confidence < $1.confidence }
                return $0.resource.path < $1.resource.path
            }
    }

    /// True when inferred ID is a prefix/parent/child of an installed ID (e.g. helper extensions).
    public func isRelated(to inferredID: String, installedIDs: Set<String>) -> Bool {
        let id = inferredID.lowercased()
        for installed in installedIDs {
            if id == installed { return true }
            if id.hasPrefix(installed + ".") { return true }
            if installed.hasPrefix(id + ".") { return true }
            // Same vendor + shared first three labels often means sibling product family still installed
            let a = id.split(separator: ".")
            let b = installed.split(separator: ".")
            if a.count >= 3 && b.count >= 3 && a.prefix(3) == b.prefix(3) {
                return true
            }
        }
        return false
    }

    public func inferBundleID(from name: String) -> String? {
        var candidate = name
        let suffixes = [".plist", ".savedState", ".savedstate", ".log", ".crash", ".ips"]
        for suffix in suffixes {
            if candidate.lowercased().hasSuffix(suffix.lowercased()) {
                candidate = String(candidate.dropLast(suffix.count))
                break
            }
        }
        if candidate.range(of: #"^[A-Z0-9]{8,12}\."#, options: .regularExpression) != nil {
            let rest = candidate.split(separator: ".").dropFirst().joined(separator: ".")
            if rest.split(separator: ".").count >= 2 {
                return rest
            }
        }

        let parts = candidate.split(separator: ".")
        guard parts.count >= 3 else { return nil }
        guard parts[0].rangeOfCharacter(from: CharacterSet.letters) != nil else { return nil }
        return candidate
    }
}
