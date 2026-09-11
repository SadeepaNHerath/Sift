import Foundation

/// High-level facade coordinating identify → discover → attribute → remove.
public final class UninstallService: @unchecked Sendable {
    public static let shared = UninstallService()

    public let inventory: AppInventoryScanner
    public let discovery: ResourceDiscoveryEngine
    public let attribution: ResourceAttributionEngine
    public let orphans: OrphanSearchEngine
    public let removal: RemovalEngine

    public init(
        inventory: AppInventoryScanner = .shared,
        discovery: ResourceDiscoveryEngine = .shared,
        attribution: ResourceAttributionEngine = .shared,
        orphans: OrphanSearchEngine = .shared,
        removal: RemovalEngine = .shared
    ) {
        self.inventory = inventory
        self.discovery = discovery
        self.attribution = attribution
        self.orphans = orphans
        self.removal = removal
    }

    public func listApplications() -> [AppIdentity] {
        inventory.scanInstalledApps()
    }

    public func analyze(app: AppIdentity, installedApps: [AppIdentity]? = nil) -> AttributionResult {
        let apps = installedApps ?? inventory.scanInstalledApps()
        let candidates = discovery.discoverCandidates(for: app)
        let attributed = attribution.attribute(candidates: candidates, to: app, installedApps: apps)
        return AttributionResult(app: app, resources: attributed)
    }

    public func analyze(appAt url: URL) -> AttributionResult? {
        guard let app = inventory.identify(at: url) else { return nil }
        return analyze(app: app)
    }

    public func findOrphanedFiles(installedApps: [AppIdentity]? = nil) -> [OrphanedResource] {
        let apps = installedApps ?? inventory.scanInstalledApps()
        return orphans.findOrphans(installedApps: apps)
    }

    public func uninstall(
        result: AttributionResult,
        selectedOverrides: [String: Bool]? = nil,
        dryRun: Bool = false
    ) async -> AuditReport {
        var resources = result.resources
        if let overrides = selectedOverrides {
            resources = resources.map { item in
                var copy = item
                if let selected = overrides[item.resource.path] {
                    copy.isSelected = selected
                }
                return copy
            }
        }
        return await removal.remove(app: result.app, resources: resources, dryRun: dryRun)
    }

    /// Probes a documented TCC-protected path (not Mail/Safari) to detect Full Disk Access.
    /// Uses `~/Library/Application Support/com.apple.TCC` when present; otherwise
    /// `~/Library/Shortcuts` listing, which is FDA-gated on modern macOS.
    public func hasFullDiskAccess() -> Bool {
        let home = FileManager.default.homeDirectoryForCurrentUser
        let tccUser = home.appendingPathComponent("Library/Application Support/com.apple.TCC")
        if FileManager.default.fileExists(atPath: tccUser.path) {
            return (try? FileManager.default.contentsOfDirectory(atPath: tccUser.path)) != nil
        }
        let shortcuts = home.appendingPathComponent("Library/Shortcuts")
        if FileManager.default.fileExists(atPath: shortcuts.path) {
            return (try? FileManager.default.contentsOfDirectory(atPath: shortcuts.path)) != nil
        }
        // Conservative: if neither path exists, treat as not granted so Setup still prompts.
        return false
    }
}
