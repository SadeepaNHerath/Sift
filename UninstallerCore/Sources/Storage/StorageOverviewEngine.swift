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

        let categories: [StorageCategorySize] = [
            StorageCategorySize(kind: .applications, byteSize: sizeOfDirectory(at: URL(fileURLWithPath: "/Applications"), maxItems: 800)),
            StorageCategorySize(kind: .caches, byteSize: sizeOfDirectory(at: home.appendingPathComponent("Library/Caches"), maxItems: 4_000)),
            StorageCategorySize(kind: .downloads, byteSize: sizeOfDirectory(at: home.appendingPathComponent("Downloads"), maxItems: 3_000)),
            StorageCategorySize(kind: .movies, byteSize: sizeOfDirectory(at: home.appendingPathComponent("Movies"), maxItems: 2_000)),
            StorageCategorySize(kind: .developer, byteSize: developerFootprint(home: home)),
            StorageCategorySize(kind: .trash, byteSize: trashSize())
        ]

        return StorageOverview(
            totalBytes: total,
            availableBytes: available,
            categories: categories.sorted { $0.byteSize > $1.byteSize }
        )
    }

    public func trashSize() -> Int64 {
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

    private func developerFootprint(home: URL) -> Int64 {
        let roots = [
            home.appendingPathComponent("Library/Developer"),
            home.appendingPathComponent(".npm"),
            home.appendingPathComponent(".gradle/caches"),
            home.appendingPathComponent(".m2/repository"),
            home.appendingPathComponent("Library/Caches/Homebrew")
        ]
        return roots.reduce(0) { $0 + sizeOfDirectory(at: $1, maxItems: 2_500) }
    }

    /// Caps traversal so Overview stays responsive on large disks.
    public func sizeOfDirectory(at url: URL, maxItems: Int = 5_000) -> Int64 {
        var isDir: ObjCBool = false
        guard fileManager.fileExists(atPath: url.path, isDirectory: &isDir) else { return 0 }
        if !isDir.boolValue {
            let values = try? url.resourceValues(forKeys: [.fileSizeKey])
            return Int64(values?.fileSize ?? 0)
        }

        var total: Int64 = 0
        var count = 0
        guard let enumerator = fileManager.enumerator(
            at: url,
            includingPropertiesForKeys: [.isRegularFileKey, .fileSizeKey, .isDirectoryKey],
            options: [.skipsHiddenFiles]
        ) else { return 0 }

        while let item = enumerator.nextObject() as? URL {
            count += 1
            if count > maxItems { break }
            if let values = try? item.resourceValues(forKeys: [.isRegularFileKey, .fileSizeKey]),
               values.isRegularFile == true {
                total += Int64(values.fileSize ?? 0)
            }
        }
        return total
    }
}
