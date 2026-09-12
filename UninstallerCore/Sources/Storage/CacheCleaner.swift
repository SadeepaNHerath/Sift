import Foundation

/// Enumerates user Library Caches leaves for safe, selective clearing.
public final class CacheCleaner: @unchecked Sendable {
    public static let shared = CacheCleaner()

    private let fileManager: FileManager
    private let denylist: SafetyDenylist
    private let overview: StorageOverviewEngine

    public init(
        fileManager: FileManager = .default,
        denylist: SafetyDenylist = .shared,
        overview: StorageOverviewEngine = .shared
    ) {
        self.fileManager = fileManager
        self.denylist = denylist
        self.overview = overview
    }

    public func scan(home: URL? = nil) -> [CacheItem] {
        let homeURL = home ?? fileManager.homeDirectoryForCurrentUser
        let cachesRoot = homeURL.appendingPathComponent("Library/Caches")
        guard fileManager.fileExists(atPath: cachesRoot.path) else { return [] }

        let children: [URL]
        do {
            children = try fileManager.contentsOfDirectory(
                at: cachesRoot,
                includingPropertiesForKeys: [.isDirectoryKey],
                options: [.skipsHiddenFiles]
            )
        } catch {
            return []
        }

        var items: [CacheItem] = []
        for child in children {
            let path = child.path
            if denylist.isDenied(path: path) { continue }
            let name = child.lastPathComponent
            if name.lowercased().hasPrefix("com.apple.") { continue }

            let size = overview.sizeOfDirectory(at: child, maxItems: 3_000).bytes
            guard size > 0 else { continue }

            let owner = inferOwner(from: name)
            let note: String
            if name.lowercased().contains("chrome") || name.lowercased().contains("firefox")
                || name.lowercased().contains("safari") || name.lowercased().contains("brave") {
                note = "Browser cache — pages may load slower until rebuilt. Does not log you out."
            } else if name.lowercased().contains("cloudkit") || name.lowercased().contains("itunes") {
                note = "May rebuild automatically; skip if unsure."
            } else {
                note = "Safe to clear; the app may recreate this cache."
            }

            items.append(
                CacheItem(
                    path: path,
                    byteSize: size,
                    inferredOwner: owner,
                    note: note,
                    isSelected: false
                )
            )
        }

        return items.sorted { $0.byteSize > $1.byteSize }
    }

    private func inferOwner(from leafName: String) -> String? {
        // Reverse-DNS leaf → last meaningful label
        let parts = leafName.split(separator: ".")
        if parts.count >= 2 {
            return String(parts.last!)
        }
        return leafName
    }
}
