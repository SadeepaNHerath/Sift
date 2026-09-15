import Foundation

/// Needs AppKit only for emptyTrash AppleScript — keep import local to RemovalEngine via @MainActor method.
import AppKit

public protocol PrivilegedRemovalClient: AnyObject {
    var isAvailable: Bool { get }
    func trashItems(at paths: [String]) async throws -> [String: String]
    func bootoutLaunchd(label: String, domain: String) async throws
}

/// Moves selected resources to Trash, unloads launchd jobs, verifies, and writes audit reports.
public final class RemovalEngine: @unchecked Sendable {
    public static let shared = RemovalEngine()

    private let fileManager: FileManager
    private let denylist: SafetyDenylist
    public var privilegedClient: PrivilegedRemovalClient?

    public init(
        fileManager: FileManager = .default,
        denylist: SafetyDenylist = .shared,
        privilegedClient: PrivilegedRemovalClient? = nil
    ) {
        self.fileManager = fileManager
        self.denylist = denylist
        self.privilegedClient = privilegedClient
    }

    public var reportsDirectory: URL {
        let support = fileManager.homeDirectoryForCurrentUser
            .appendingPathComponent("Library/Application Support/SafeUninstaller", isDirectory: true)
        let base = support.appendingPathComponent("Reports", isDirectory: true)
        try? fileManager.createDirectory(at: base, withIntermediateDirectories: true)
        // Restrict to the current user (0700).
        try? fileManager.setAttributes([.posixPermissions: 0o700], ofItemAtPath: support.path)
        try? fileManager.setAttributes([.posixPermissions: 0o700], ofItemAtPath: base.path)
        return base
    }

    public func remove(
        app: AppIdentity,
        resources: [AttributedResource],
        dryRun: Bool = false
    ) async -> AuditReport {
        var results: [RemovalItemResult] = []

        // Unload launch agents/daemons before trashing their plists.
        let launchdItems = resources.filter {
            $0.isSelected && ($0.resource.category == .launchAgents || $0.resource.category == .launchDaemons)
        }
        for item in launchdItems {
            if dryRun { continue }
            await unloadLaunchd(for: item)
        }

        for item in resources {
            if item.isDenied {
                results.append(
                    RemovalItemResult(
                        path: item.resource.path,
                        category: item.resource.category,
                        confidence: item.confidence,
                        signals: item.signals,
                        outcome: .skippedDenied,
                        errorMessage: item.denyReason,
                        byteSize: item.resource.byteSize
                    )
                )
                continue
            }

            guard item.isSelected else {
                results.append(
                    RemovalItemResult(
                        path: item.resource.path,
                        category: item.resource.category,
                        confidence: item.confidence,
                        signals: item.signals,
                        outcome: .retained,
                        byteSize: item.resource.byteSize
                    )
                )
                continue
            }

            if let reason = denylist.denyReason(for: item.resource.path) {
                results.append(
                    RemovalItemResult(
                        path: item.resource.path,
                        category: item.resource.category,
                        confidence: item.confidence,
                        signals: item.signals,
                        outcome: .skippedDenied,
                        errorMessage: reason,
                        byteSize: item.resource.byteSize
                    )
                )
                continue
            }

            if dryRun {
                results.append(
                    RemovalItemResult(
                        path: item.resource.path,
                        category: item.resource.category,
                        confidence: item.confidence,
                        signals: item.signals,
                        outcome: .removed,
                        errorMessage: "dry-run",
                        byteSize: item.resource.byteSize
                    )
                )
                continue
            }

            let outcome = await trash(item: item)
            results.append(outcome)
        }

        let report = AuditReport(
            appName: app.name,
            bundleIdentifier: app.bundleIdentifier,
            appPath: app.path,
            items: results,
            notes: dryRun ? "Dry run — nothing was moved to Trash." : "Items were moved to Trash when possible."
        )
        try? save(report: report)
        return report
    }

    public func removeOrphans(_ orphans: [OrphanedResource], dryRun: Bool = false) async -> AuditReport {
        let attributed = orphans.map { orphan in
            AttributedResource(
                resource: orphan.resource,
                signals: [],
                confidence: orphan.confidence,
                score: orphan.confidence == .high ? 0.9 : 0.5,
                isSelected: orphan.isSelected
            )
        }
        let synthetic = AppIdentity(
            path: "",
            bundleIdentifier: "orphan.search",
            name: "Orphaned Files"
        )
        return await remove(app: synthetic, resources: attributed, dryRun: dryRun)
    }

    private func trash(item: AttributedResource) async -> RemovalItemResult {
        let path = item.resource.path
        let url = URL(fileURLWithPath: path)

        if item.resource.requiresPrivilege {
            if let allowReason = PathAllowlist.privilegedTrashDenialReason(for: path) {
                return RemovalItemResult(
                    path: path,
                    category: item.resource.category,
                    confidence: item.confidence,
                    signals: item.signals,
                    outcome: .skippedDenied,
                    errorMessage: allowReason,
                    byteSize: item.resource.byteSize
                )
            }
            if let client = privilegedClient, client.isAvailable {
                do {
                    if let reason = denylist.preTrashDenialReason(for: path) {
                        return RemovalItemResult(
                            path: path,
                            category: item.resource.category,
                            confidence: item.confidence,
                            signals: item.signals,
                            outcome: .skippedDenied,
                            errorMessage: reason,
                            byteSize: item.resource.byteSize
                        )
                    }
                    let map = try await client.trashItems(at: [path])
                    if let err = map[path], !err.isEmpty {
                        return RemovalItemResult(
                            path: path,
                            category: item.resource.category,
                            confidence: item.confidence,
                            signals: item.signals,
                            outcome: .failed,
                            errorMessage: err,
                            byteSize: item.resource.byteSize
                        )
                    }
                    let gone = !fileManager.fileExists(atPath: path)
                    return RemovalItemResult(
                        path: path,
                        category: item.resource.category,
                        confidence: item.confidence,
                        signals: item.signals,
                        outcome: gone ? .removed : .failed,
                        errorMessage: gone ? nil : "Path still exists after privileged trash",
                        byteSize: item.resource.byteSize
                    )
                } catch {
                    return RemovalItemResult(
                        path: path,
                        category: item.resource.category,
                        confidence: item.confidence,
                        signals: item.signals,
                        outcome: .failed,
                        errorMessage: error.localizedDescription,
                        byteSize: item.resource.byteSize
                    )
                }
            } else {
                return RemovalItemResult(
                    path: path,
                    category: item.resource.category,
                    confidence: item.confidence,
                    signals: item.signals,
                    outcome: .requiresHelper,
                    errorMessage: "Privileged helper not available",
                    byteSize: item.resource.byteSize
                )
            }
        }

        do {
            if let reason = denylist.preTrashDenialReason(for: path) {
                return RemovalItemResult(
                    path: path,
                    category: item.resource.category,
                    confidence: item.confidence,
                    signals: item.signals,
                    outcome: .skippedDenied,
                    errorMessage: reason,
                    byteSize: item.resource.byteSize
                )
            }
            var resulting: NSURL?
            try fileManager.trashItem(at: url, resultingItemURL: &resulting)
            let gone = !fileManager.fileExists(atPath: path)
            return RemovalItemResult(
                path: path,
                category: item.resource.category,
                confidence: item.confidence,
                signals: item.signals,
                outcome: gone ? .removed : .failed,
                errorMessage: gone ? nil : "Path still exists after trash",
                byteSize: item.resource.byteSize
            )
        } catch {
            return RemovalItemResult(
                path: path,
                category: item.resource.category,
                confidence: item.confidence,
                signals: item.signals,
                outcome: .failed,
                errorMessage: error.localizedDescription,
                byteSize: item.resource.byteSize
            )
        }
    }

    private func unloadLaunchd(for item: AttributedResource) async {
        let label = launchdLabel(fromPlist: item.resource.path) ?? (item.resource.path as NSString).lastPathComponent
            .replacingOccurrences(of: ".plist", with: "")
        let domain: String
        if item.resource.isSystemLocation {
            domain = "system"
            if let client = privilegedClient, client.isAvailable {
                try? await client.bootoutLaunchd(label: label, domain: domain)
            }
        } else {
            domain = "gui/\(getuid())"
            _ = TimedProcess.run(
                executable: "/bin/launchctl",
                arguments: ["bootout", domain, item.resource.path],
                timeout: 8
            )
        }
    }

    private func launchdLabel(fromPlist path: String) -> String? {
        guard let dict = NSDictionary(contentsOfFile: path) as? [String: Any] else { return nil }
        return dict["Label"] as? String
    }

    public func save(report: AuditReport) throws {
        let formatter = ISO8601DateFormatter()
        formatter.formatOptions = [.withInternetDateTime]
        let stamp = formatter.string(from: report.createdAt).replacingOccurrences(of: ":", with: "-")
        let baseName = "\(stamp)_\(report.appName.replacingOccurrences(of: "/", with: "-"))"

        let jsonURL = reportsDirectory.appendingPathComponent(baseName + ".json")
        let txtURL = reportsDirectory.appendingPathComponent(baseName + ".txt")

        let encoder = JSONEncoder()
        encoder.outputFormatting = [.prettyPrinted, .sortedKeys]
        encoder.dateEncodingStrategy = .iso8601
        let data = try encoder.encode(report)
        try data.write(to: jsonURL, options: .atomic)
        try report.humanReadableSummary.write(to: txtURL, atomically: true, encoding: .utf8)
        try? fileManager.setAttributes([.posixPermissions: 0o600], ofItemAtPath: jsonURL.path)
        try? fileManager.setAttributes([.posixPermissions: 0o600], ofItemAtPath: txtURL.path)
    }

    public func loadReports() -> [AuditReport] {
        let urls: [URL]
        do {
            urls = try fileManager.contentsOfDirectory(at: reportsDirectory, includingPropertiesForKeys: nil)
                .filter { $0.pathExtension == "json" }
                .sorted { $0.lastPathComponent > $1.lastPathComponent }
        } catch {
            return []
        }
        let decoder = JSONDecoder()
        decoder.dateDecodingStrategy = .iso8601
        return urls.compactMap { url in
            guard let data = try? Data(contentsOf: url) else { return nil }
            return try? decoder.decode(AuditReport.self, from: data)
        }
    }

    /// Trash arbitrary user-selected paths (caches, large files, dev deps) with denylist checks.
    /// Only home-directory (or system temp) paths are accepted — defense if UI is bypassed.
    public func trashPaths(
        _ paths: [(path: String, category: ResourceCategory, byteSize: Int64)],
        reportName: String,
        dryRun: Bool = false
    ) async -> AuditReport {
        let home = (fileManager.homeDirectoryForCurrentUser.path as NSString).standardizingPath
        let tmp = (NSTemporaryDirectory() as NSString).standardizingPath
        var attributed: [AttributedResource] = []
        for entry in paths {
            let standardized = (entry.path as NSString).standardizingPath
            let underHome = standardized == home || standardized.hasPrefix(home + "/")
            let underTmp = standardized == tmp || standardized.hasPrefix(tmp + "/")
            guard underHome || underTmp else {
                attributed.append(
                    AttributedResource(
                        resource: DiscoveredResource(
                            path: entry.path,
                            category: entry.category,
                            isDirectory: true,
                            byteSize: entry.byteSize
                        ),
                        signals: [],
                        confidence: .low,
                        score: 0,
                        isDenied: true,
                        denyReason: "Path outside home/temp — refused",
                        isSelected: false
                    )
                )
                continue
            }
            if let reason = denylist.preTrashDenialReason(for: entry.path) {
                attributed.append(
                    AttributedResource(
                        resource: DiscoveredResource(
                            path: entry.path,
                            category: entry.category,
                            isDirectory: true,
                            byteSize: entry.byteSize
                        ),
                        signals: [],
                        confidence: .low,
                        score: 0,
                        isDenied: true,
                        denyReason: reason,
                        isSelected: false
                    )
                )
                continue
            }
            let resource = DiscoveredResource(
                path: entry.path,
                category: entry.category,
                isDirectory: true,
                byteSize: entry.byteSize
            )
            attributed.append(
                AttributedResource(
                    resource: resource,
                    signals: [],
                    confidence: .medium,
                    score: 0.5,
                    isSelected: true
                )
            )
        }
        let synthetic = AppIdentity(
            path: "",
            bundleIdentifier: "storage.toolkit",
            name: reportName
        )
        return await remove(app: synthetic, resources: attributed, dryRun: dryRun)
    }

    /// Empties the user's Trash via Finder. Returns an error message on failure.
    @MainActor
    public func emptyTrash() -> String? {
        let script = """
        tell application "Finder"
            empty the trash
        end tell
        """
        var error: NSDictionary?
        if let appleScript = NSAppleScript(source: script) {
            appleScript.executeAndReturnError(&error)
            if let error {
                return error[NSAppleScript.errorMessage] as? String ?? "Could not empty Trash"
            }
            return nil
        }
        return "Could not create AppleScript"
    }
}
