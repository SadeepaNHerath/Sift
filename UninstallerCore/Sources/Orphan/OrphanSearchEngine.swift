import Foundation

/// Finds leftover files whose inferred ownership does not match any currently installed application.
public final class OrphanSearchEngine: @unchecked Sendable {
    public static let shared = OrphanSearchEngine()

    /// Product families that share identity across different reverse-DNS prefixes.
    /// When any member is installed, leftovers for every member are suppressed.
    public static let knownFamilies: [[String]] = [
        ["net.whatsapp", "com.facebook", "com.instagram", "com.whatsapp"]
    ]

    /// System / OS leftovers that must never appear as uninstall candidates.
    public static let systemPrefixes: [String] = [
        "com.apple.",
        "org.cups.",
        "is.workflow."
    ]

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
        let installedVendors = Set(installedIDs.compactMap { Self.vendorPrefix(from: $0) })

        let entries = discovery.enumerateAllShallowEntries(searchPaths: searchPaths)
        var orphans: [OrphanedResource] = []

        for entry in entries {
            if denylist.isDenied(path: entry.path) { continue }

            let name = (entry.path as NSString).lastPathComponent
            let lower = name.lowercased()

            if let inferredID = inferBundleID(from: name) {
                if shouldSkipInferredID(inferredID, installedIDs: installedIDs, installedVendors: installedVendors) {
                    continue
                }
                orphans.append(
                    OrphanedResource(
                        resource: entry,
                        inferredBundleID: Self.normalizeBundleID(inferredID),
                        inferredName: nil,
                        confidence: .low,
                        isSelected: false
                    )
                )
                continue
            }

            if entry.category == .savedState, lower.hasSuffix(".savedstate") {
                let base = String(lower.dropLast(".savedstate".count))
                if shouldSkipInferredID(base, installedIDs: installedIDs, installedVendors: installedVendors) {
                    continue
                }
                orphans.append(
                    OrphanedResource(
                        resource: entry,
                        inferredBundleID: base.contains(".") ? Self.normalizeBundleID(base) : nil,
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
                    if shouldSkipInferredID(id, installedIDs: installedIDs, installedVendors: installedVendors) {
                        continue
                    }
                    orphans.append(
                        OrphanedResource(
                            resource: entry,
                            inferredBundleID: Self.normalizeBundleID(id),
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

    /// Strip `group.` and lowercase for matching against installed apps.
    public static func normalizeBundleID(_ raw: String) -> String {
        var id = raw.lowercased()
        if id.hasPrefix("group.") {
            id = String(id.dropFirst("group.".count))
        }
        return id
    }

    /// First two reverse-DNS labels (`net.whatsapp`, `com.docker`).
    public static func vendorPrefix(from bundleID: String) -> String? {
        let id = normalizeBundleID(bundleID)
        let parts = id.split(separator: ".")
        guard parts.count >= 2 else { return nil }
        return "\(parts[0]).\(parts[1])"
    }

    public func shouldSkipInferredID(
        _ inferredID: String,
        installedIDs: Set<String>,
        installedVendors: Set<String>
    ) -> Bool {
        let id = Self.normalizeBundleID(inferredID)
        if Self.systemPrefixes.contains(where: { id.hasPrefix($0) || id == String($0.dropLast()) }) {
            return true
        }
        if installedIDs.contains(id) { return true }
        if isRelated(to: id, installedIDs: installedIDs) { return true }
        if let vendor = Self.vendorPrefix(from: id), installedVendors.contains(vendor) {
            return true
        }
        if Self.familyStillInstalled(for: id, installedIDs: installedIDs, installedVendors: installedVendors) {
            return true
        }
        return false
    }

    public static func familyStillInstalled(
        for inferredID: String,
        installedIDs: Set<String>,
        installedVendors: Set<String>
    ) -> Bool {
        let id = normalizeBundleID(inferredID)
        guard let vendor = vendorPrefix(from: id) else { return false }
        for family in knownFamilies where family.contains(vendor) {
            for member in family {
                if installedVendors.contains(member) { return true }
                if installedIDs.contains(where: { $0 == member || $0.hasPrefix(member + ".") }) {
                    return true
                }
            }
        }
        return false
    }

    /// True when inferred ID is a prefix/parent/child of an installed ID (e.g. helper extensions).
    public func isRelated(to inferredID: String, installedIDs: Set<String>) -> Bool {
        let id = Self.normalizeBundleID(inferredID)
        for installedRaw in installedIDs {
            let installed = Self.normalizeBundleID(installedRaw)
            if id == installed { return true }
            if id.hasPrefix(installed + ".") { return true }
            if installed.hasPrefix(id + ".") { return true }
            // Same vendor + shared first three labels often means sibling product family still installed
            let a = id.split(separator: ".")
            let b = installed.split(separator: ".")
            if a.count >= 3 && b.count >= 3 && a.prefix(3) == b.prefix(3) {
                return true
            }
            // Two-label vendor match (net.whatsapp.* vs net.whatsapp.WhatsApp)
            if a.count >= 2 && b.count >= 2 && a.prefix(2) == b.prefix(2) {
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
        // Allow group.net.whatsapp… (4+ parts) and classic reverse-DNS (3+)
        guard parts.count >= 3 else { return nil }
        guard parts[0].rangeOfCharacter(from: CharacterSet.letters) != nil else { return nil }
        return candidate
    }
}
