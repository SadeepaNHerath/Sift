import Foundation

/// Known macOS locations where applications commonly store related files.
public struct DiscoveryPath: Sendable {
    public let url: URL
    public let category: ResourceCategory
    public let isSystemLocation: Bool
    public let requiresPrivilege: Bool
    public let shallow: Bool

    public init(
        url: URL,
        category: ResourceCategory,
        isSystemLocation: Bool = false,
        requiresPrivilege: Bool = false,
        shallow: Bool = true
    ) {
        self.url = url
        self.category = category
        self.isSystemLocation = isSystemLocation
        self.requiresPrivilege = requiresPrivilege
        self.shallow = shallow
    }
}

public final class ResourceDiscoveryEngine: @unchecked Sendable {
    public static let shared = ResourceDiscoveryEngine()

    private let fileManager: FileManager

    public init(fileManager: FileManager = .default) {
        self.fileManager = fileManager
    }

    public func standardSearchPaths(home: URL? = nil) -> [DiscoveryPath] {
        let homeURL = home ?? fileManager.homeDirectoryForCurrentUser
        let userLibrary = homeURL.appendingPathComponent("Library")

        var paths: [DiscoveryPath] = [
            DiscoveryPath(url: userLibrary.appendingPathComponent("Application Support"), category: .applicationSupport),
            DiscoveryPath(url: userLibrary.appendingPathComponent("Caches"), category: .caches),
            DiscoveryPath(url: userLibrary.appendingPathComponent("Preferences"), category: .preferences),
            DiscoveryPath(url: userLibrary.appendingPathComponent("Saved Application State"), category: .savedState),
            DiscoveryPath(url: userLibrary.appendingPathComponent("Containers"), category: .containers),
            DiscoveryPath(url: userLibrary.appendingPathComponent("Group Containers"), category: .groupContainers),
            DiscoveryPath(url: userLibrary.appendingPathComponent("Logs"), category: .logs),
            DiscoveryPath(url: userLibrary.appendingPathComponent("Application Scripts"), category: .applicationScripts),
            DiscoveryPath(url: userLibrary.appendingPathComponent("WebKit"), category: .webKit),
            DiscoveryPath(url: userLibrary.appendingPathComponent("HTTPStorages"), category: .httpStorages),
            DiscoveryPath(url: userLibrary.appendingPathComponent("LaunchAgents"), category: .launchAgents),
            DiscoveryPath(url: userLibrary.appendingPathComponent("Logs/DiagnosticReports"), category: .crashReports),
            DiscoveryPath(
                url: URL(fileURLWithPath: "/Library/Application Support"),
                category: .applicationSupport,
                isSystemLocation: true,
                requiresPrivilege: true
            ),
            DiscoveryPath(
                url: URL(fileURLWithPath: "/Library/Caches"),
                category: .caches,
                isSystemLocation: true,
                requiresPrivilege: true
            ),
            DiscoveryPath(
                url: URL(fileURLWithPath: "/Library/Preferences"),
                category: .preferences,
                isSystemLocation: true,
                requiresPrivilege: true
            ),
            DiscoveryPath(
                url: URL(fileURLWithPath: "/Library/LaunchAgents"),
                category: .launchAgents,
                isSystemLocation: true,
                requiresPrivilege: true
            ),
            DiscoveryPath(
                url: URL(fileURLWithPath: "/Library/LaunchDaemons"),
                category: .launchDaemons,
                isSystemLocation: true,
                requiresPrivilege: true
            ),
            DiscoveryPath(
                url: URL(fileURLWithPath: "/Library/PrivilegedHelperTools"),
                category: .privilegedHelpers,
                isSystemLocation: true,
                requiresPrivilege: true
            ),
            DiscoveryPath(
                url: URL(fileURLWithPath: "/Library/Logs/DiagnosticReports"),
                category: .crashReports,
                isSystemLocation: true,
                requiresPrivilege: false
            )
        ]
        return paths
    }

    /// Collects candidate entries under known roots that might relate to `app`.
    /// Filtering/attribution is performed separately.
    public func discoverCandidates(
        for app: AppIdentity,
        searchPaths: [DiscoveryPath]? = nil,
        includeAppBundle: Bool = true
    ) -> [DiscoveredResource] {
        let roots = searchPaths ?? standardSearchPaths()
        var results: [DiscoveredResource] = []
        var seen = Set<String>()

        if includeAppBundle {
            let appResource = makeResource(
                at: URL(fileURLWithPath: app.path),
                category: .applicationBundle,
                isSystemLocation: app.path.hasPrefix("/Applications") == false && app.path.hasPrefix(fileManager.homeDirectoryForCurrentUser.path) == false
                    ? false
                    : !app.path.hasPrefix(fileManager.homeDirectoryForCurrentUser.path) && !app.path.hasPrefix("/Users"),
                requiresPrivilege: false
            )
            if let appResource, seen.insert(appResource.path).inserted {
                results.append(appResource)
            }
        }

        let needles = searchNeedles(for: app)

        for root in roots {
            guard fileManager.fileExists(atPath: root.url.path) else { continue }
            let children: [URL]
            do {
                children = try fileManager.contentsOfDirectory(
                    at: root.url,
                    includingPropertiesForKeys: [.isDirectoryKey, .fileSizeKey, .isSymbolicLinkKey],
                    options: [.skipsHiddenFiles]
                )
            } catch {
                continue
            }

            for child in children {
                let name = child.lastPathComponent
                let lower = name.lowercased()
                let pathLower = child.path.lowercased()

                let matches = needles.contains { needle in
                    Self.pathMatchesNeedle(nameLower: lower, pathLower: pathLower, needle: needle)
                }
                guard matches else { continue }

                if let resource = makeResource(
                    at: child,
                    category: root.category,
                    isSystemLocation: root.isSystemLocation,
                    requiresPrivilege: root.requiresPrivilege
                ), seen.insert(resource.path).inserted {
                    results.append(resource)
                }
            }
        }

        // pkg receipt files (paths listed by pkgutil) — best-effort
        for path in pkgReceiptPaths(for: app) {
            let url = URL(fileURLWithPath: path)
            guard fileManager.fileExists(atPath: path) else { continue }
            if let resource = makeResource(
                at: url,
                category: .other,
                isSystemLocation: !path.hasPrefix(fileManager.homeDirectoryForCurrentUser.path),
                requiresPrivilege: path.hasPrefix("/Library") || path.hasPrefix("/usr")
            ), seen.insert(resource.path).inserted {
                results.append(resource)
            }
        }

        return results
    }

    /// Enumerate shallow children of all roots without filtering (used by orphan search).
    public func enumerateAllShallowEntries(searchPaths: [DiscoveryPath]? = nil) -> [DiscoveredResource] {
        let roots = searchPaths ?? standardSearchPaths()
        var results: [DiscoveredResource] = []
        var seen = Set<String>()

        for root in roots {
            guard fileManager.fileExists(atPath: root.url.path) else { continue }
            let children: [URL]
            do {
                children = try fileManager.contentsOfDirectory(
                    at: root.url,
                    includingPropertiesForKeys: [.isDirectoryKey, .fileSizeKey],
                    options: [.skipsHiddenFiles]
                )
            } catch {
                continue
            }
            for child in children {
                if let resource = makeResource(
                    at: child,
                    category: root.category,
                    isSystemLocation: root.isSystemLocation,
                    requiresPrivilege: root.requiresPrivilege
                ), seen.insert(resource.path).inserted {
                    results.append(resource)
                }
            }
        }
        return results
    }

    public func searchNeedles(for app: AppIdentity) -> [String] {
        var needles = Set<String>()
        if !app.bundleIdentifier.isEmpty {
            needles.insert(app.bundleIdentifier.lowercased())
        }
        if let vendor = app.vendorPrefix, !vendor.isEmpty {
            needles.insert(vendor.lowercased())
        }
        let name = app.name.lowercased()
        if !name.isEmpty {
            needles.insert(name)
            needles.insert(name.replacingOccurrences(of: " ", with: ""))
            needles.insert(name.replacingOccurrences(of: " ", with: "-"))
            needles.insert(name.replacingOccurrences(of: " ", with: "_"))
        }
        if !app.executableName.isEmpty {
            needles.insert(app.executableName.lowercased())
        }
        if !app.normalizedName.isEmpty {
            needles.insert(app.normalizedName)
        }
        return Array(needles).filter { $0.count >= 3 }
    }

    /// Short needles (< 6 chars) must match a path component or filename boundary to avoid
    /// false positives like "Box" matching "Sandbox".
    public static func pathMatchesNeedle(nameLower: String, pathLower: String, needle: String) -> Bool {
        let n = needle.lowercased()
        guard !n.isEmpty else { return false }
        if n.count >= 6 {
            return nameLower.contains(n) || pathLower.contains(n)
        }
        if nameLower == n { return true }
        if nameLower.hasPrefix(n + ".") || nameLower.hasPrefix(n + "-") || nameLower.hasPrefix(n + "_") {
            return true
        }
        if nameLower.hasSuffix("." + n) || nameLower.hasSuffix("-" + n) || nameLower.hasSuffix("_" + n) {
            return true
        }
        let components = pathLower.split(separator: "/").map(String.init)
        return components.contains { component in
            if component == n { return true }
            if component.hasPrefix(n + ".") || component.hasPrefix(n + "-") || component.hasPrefix(n + "_") {
                return true
            }
            return PathAllowlist.pathMatchesBundleID("/" + component, bundleID: n)
        }
    }

    public func makeResource(
        at url: URL,
        category: ResourceCategory,
        isSystemLocation: Bool,
        requiresPrivilege: Bool
    ) -> DiscoveredResource? {
        var isDir: ObjCBool = false
        guard fileManager.fileExists(atPath: url.path, isDirectory: &isDir) else { return nil }
        let size = directoryByteSize(at: url)
        return DiscoveredResource(
            path: url.path,
            category: category,
            isDirectory: isDir.boolValue,
            byteSize: size,
            isSystemLocation: isSystemLocation,
            requiresPrivilege: requiresPrivilege
        )
    }

    private func directoryByteSize(at url: URL) -> Int64 {
        var isDir: ObjCBool = false
        guard fileManager.fileExists(atPath: url.path, isDirectory: &isDir) else { return 0 }
        if !isDir.boolValue {
            let values = try? url.resourceValues(forKeys: [.fileSizeKey])
            return Int64(values?.fileSize ?? 0)
        }

        // Cap traversal for large trees.
        var total: Int64 = 0
        var count = 0
        let enumerator = fileManager.enumerator(at: url, includingPropertiesForKeys: [.fileSizeKey, .isRegularFileKey], options: [.skipsHiddenFiles])
        while let item = enumerator?.nextObject() as? URL {
            count += 1
            if count > 5_000 { break }
            if let values = try? item.resourceValues(forKeys: [.isRegularFileKey, .fileSizeKey]),
               values.isRegularFile == true {
                total += Int64(values.fileSize ?? 0)
            }
        }
        return total
    }

    /// Best-effort: find pkg IDs that look related and return their file lists.
    public func pkgReceiptPaths(for app: AppIdentity) -> [String] {
        guard !app.bundleIdentifier.isEmpty else { return [] }
        let pkgsResult = TimedProcess.run(
            executable: "/usr/sbin/pkgutil",
            arguments: ["--pkgs"],
            timeout: 10
        )
        guard !pkgsResult.timedOut, let output = String(data: pkgsResult.stdout, encoding: .utf8) else {
            return []
        }
        let pkgs = output.split(separator: "\n").map(String.init)
        let related = pkgs.filter {
            $0.localizedCaseInsensitiveContains(app.bundleIdentifier)
                || ($0.localizedCaseInsensitiveContains(app.vendorPrefix ?? "___never___"))
        }
        var paths: [String] = []
        for pkg in related.prefix(5) {
            let listResult = TimedProcess.run(
                executable: "/usr/sbin/pkgutil",
                arguments: ["--files", pkg],
                timeout: 10
            )
            guard !listResult.timedOut, let text = String(data: listResult.stdout, encoding: .utf8) else {
                continue
            }
            // pkgutil --files returns relative paths; absolute root unknown — skip unless absolute.
            for line in text.split(separator: "\n").map(String.init) where line.hasPrefix("/") {
                paths.append(line)
            }
        }
        return paths
    }
}
