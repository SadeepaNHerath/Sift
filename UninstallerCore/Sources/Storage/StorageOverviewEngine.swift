import Foundation

/// Best-effort volume + category size overview for the low-storage dashboard.
public final class StorageOverviewEngine: @unchecked Sendable {
    public static let shared = StorageOverviewEngine()

    private let fileManager: FileManager

    public init(fileManager: FileManager = .default) {
        self.fileManager = fileManager
    }

    public func snapshot() -> StorageOverview {
        let home = fileManager.homeDirectoryForCurrentUser
        let (total, available) = volumeCapacity(for: home)

        let apps = sizeOfDirectory(at: URL(fileURLWithPath: "/Applications"), maxItems: 800)
        let caches = sizeOfDirectory(at: home.appendingPathComponent("Library/Caches"), maxItems: 4_000)
        let downloads = sizeOfDirectory(at: home.appendingPathComponent("Downloads"), maxItems: 3_000)
        let movies = sizeOfDirectory(at: home.appendingPathComponent("Movies"), maxItems: 2_000)
        let developer = developerFootprint(home: home)
        let trash = trashSizeDetailed()

        let categories: [StorageCategorySize] = [
            StorageCategorySize(kind: .applications, byteSize: apps.bytes, isApproximate: apps.isApproximate),
            StorageCategorySize(kind: .caches, byteSize: caches.bytes, isApproximate: caches.isApproximate),
            StorageCategorySize(kind: .downloads, byteSize: downloads.bytes, isApproximate: downloads.isApproximate),
            StorageCategorySize(kind: .movies, byteSize: movies.bytes, isApproximate: movies.isApproximate),
            StorageCategorySize(kind: .developer, byteSize: developer.bytes, isApproximate: developer.isApproximate),
            StorageCategorySize(kind: .trash, byteSize: trash.bytes, isApproximate: trash.isApproximate)
        ]

        return StorageOverview(
            totalBytes: total,
            availableBytes: available,
            categories: categories.sorted { $0.byteSize > $1.byteSize }
        )
    }

    public func trashSize() -> Int64 {
        trashSizeDetailed().bytes
    }

    private func trashSizeDetailed() -> (bytes: Int64, isApproximate: Bool) {
        let trash = (NSHomeDirectory() as NSString).appendingPathComponent(".Trash")
        return sizeOfDirectory(at: URL(fileURLWithPath: trash), maxItems: 5_000)
    }

    private func volumeCapacity(for url: URL) -> (total: Int64, available: Int64) {
        let values = try? url.resourceValues(forKeys: [
            .volumeTotalCapacityKey,
            .volumeAvailableCapacityForImportantUsageKey,
            .volumeAvailableCapacityKey
        ])
        let total = Int64(values?.volumeTotalCapacity ?? 0)
        let availableImportant = values?.volumeAvailableCapacityForImportantUsage ?? 0
        let availableFallback = Int64(values?.volumeAvailableCapacity ?? 0)
        let available = availableImportant > 0 ? availableImportant : availableFallback
        return (total, available)
    }

    private func developerFootprint(home: URL) -> (bytes: Int64, isApproximate: Bool) {
        let roots = [
            home.appendingPathComponent("Library/Developer"),
            home.appendingPathComponent(".npm"),
            home.appendingPathComponent(".gradle/caches"),
            home.appendingPathComponent(".m2/repository"),
            home.appendingPathComponent("Library/Caches/Homebrew")
        ]
        var total: Int64 = 0
        var approx = false
        for root in roots {
            let result = sizeOfDirectory(at: root, maxItems: 2_500)
            total += result.bytes
            if result.isApproximate { approx = true }
        }
        return (total, approx)
    }

    /// Caps traversal so Overview stays responsive on large disks.
    public func sizeOfDirectory(at url: URL, maxItems: Int = 5_000) -> (bytes: Int64, isApproximate: Bool) {
        var isDir: ObjCBool = false
        guard fileManager.fileExists(atPath: url.path, isDirectory: &isDir) else { return (0, false) }
        if !isDir.boolValue {
            let values = try? url.resourceValues(forKeys: [.fileSizeKey])
            return (Int64(values?.fileSize ?? 0), false)
        }

        var total: Int64 = 0
        var count = 0
        var hitCap = false
        guard let enumerator = fileManager.enumerator(
            at: url,
            includingPropertiesForKeys: [.isRegularFileKey, .fileSizeKey, .isDirectoryKey],
            options: [.skipsHiddenFiles]
        ) else { return (0, false) }

        while let item = enumerator.nextObject() as? URL {
            count += 1
            if count > maxItems {
                hitCap = true
                break
            }
            if let values = try? item.resourceValues(forKeys: [.isRegularFileKey, .fileSizeKey]),
               values.isRegularFile == true {
                total += Int64(values.fileSize ?? 0)
            }
        }
        return (total, hitCap)
    }
}
