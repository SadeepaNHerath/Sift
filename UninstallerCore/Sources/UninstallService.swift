import Foundation

/// High-level facade coordinating identify → discover → attribute → remove + storage toolkit.
public final class UninstallService: @unchecked Sendable {
    public static let shared = UninstallService()

    public let inventory: AppInventoryScanner
    public let discovery: ResourceDiscoveryEngine
    public let attribution: ResourceAttributionEngine
    public let orphans: OrphanSearchEngine
    public let removal: RemovalEngine
    public let storageOverview: StorageOverviewEngine
    public let largeFiles: LargeFileScanner
    public let videoCompressor: VideoCompressor
    public let imageCompressor: ImageCompressor
    public let caches: CacheCleaner
    public let devDependencies: DevDependencyScanner
    public let cliCleanup: CLICleanupRunner

    public init(
        inventory: AppInventoryScanner = .shared,
        discovery: ResourceDiscoveryEngine = .shared,
        attribution: ResourceAttributionEngine = .shared,
        orphans: OrphanSearchEngine = .shared,
        removal: RemovalEngine = .shared,
        storageOverview: StorageOverviewEngine = .shared,
        largeFiles: LargeFileScanner = .shared,
        videoCompressor: VideoCompressor = .shared,
        imageCompressor: ImageCompressor = .shared,
        caches: CacheCleaner = .shared,
        devDependencies: DevDependencyScanner = .shared,
        cliCleanup: CLICleanupRunner = .shared
    ) {
        self.inventory = inventory
        self.discovery = discovery
        self.attribution = attribution
        self.orphans = orphans
        self.removal = removal
        self.storageOverview = storageOverview
        self.largeFiles = largeFiles
        self.videoCompressor = videoCompressor
        self.imageCompressor = imageCompressor
        self.caches = caches
        self.devDependencies = devDependencies
        self.cliCleanup = cliCleanup
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

    public func storageSnapshot() -> StorageOverview {
        storageOverview.snapshot()
    }

    /// Deeper remeasure used after the first Overview paint.
    public func storageSnapshotDeep() -> StorageOverview {
        storageOverview.snapshot(maxItems: 100_000, timeBudget: 5.0)
    }

    public func scanLargeFiles(
        minimumBytes: Int64 = 100 * 1024 * 1024,
        includeExpandedHome: Bool? = nil
    ) -> [LargeFileItem] {
        let expand = includeExpandedHome ?? hasFullDiskAccess()
        return largeFiles.scan(minimumBytes: minimumBytes, includeExpandedHome: expand)
    }

    public func scanCaches() -> [CacheItem] {
        caches.scan()
    }

    public func scanDevDependencies() -> [DevDependencyItem] {
        devDependencies.scan()
    }

    /// Probes a documented TCC-protected path (not Mail/Safari) to detect Full Disk Access.
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
        return false
    }
}
