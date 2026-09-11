import Foundation
import CoreServices

/// Finds large files under common user media folders.
/// Prefers Spotlight (`MDQuery`) when the index responds; falls back to FileManager enumeration.
public final class LargeFileScanner: @unchecked Sendable {
    public static let shared = LargeFileScanner()

    public static let videoExtensions: Set<String> = ["mov", "mp4", "m4v", "avi", "mkv", "mpg", "mpeg", "wmv"]
    public static let imageExtensions: Set<String> = ["jpg", "jpeg", "png", "heic", "heif", "tiff", "tif"]

    private let fileManager: FileManager
    private let denylist: SafetyDenylist

    public init(fileManager: FileManager = .default, denylist: SafetyDenylist = .shared) {
        self.fileManager = fileManager
        self.denylist = denylist
    }

    public func defaultRoots(home: URL? = nil) -> [URL] {
        let homeURL = home ?? fileManager.homeDirectoryForCurrentUser
        return [
            homeURL.appendingPathComponent("Downloads"),
            homeURL.appendingPathComponent("Movies"),
            homeURL.appendingPathComponent("Desktop"),
            homeURL.appendingPathComponent("Documents")
        ]
    }

    public func scan(
        roots: [URL]? = nil,
        minimumBytes: Int64 = 100 * 1024 * 1024,
        maxFiles: Int = 5_000,
        preferSpotlight: Bool = true
    ) -> [LargeFileItem] {
        let searchRoots = roots ?? defaultRoots()

        if preferSpotlight {
            let spotlight = spotlightScan(roots: searchRoots, minimumBytes: minimumBytes, maxFiles: maxFiles)
            if !spotlight.isEmpty {
                return spotlight.sorted { $0.byteSize > $1.byteSize }
            }
        }

        return fileManagerScan(roots: searchRoots, minimumBytes: minimumBytes, maxFiles: maxFiles)
            .sorted { $0.byteSize > $1.byteSize }
    }

    // MARK: - Spotlight (MDQuery)

    private func spotlightScan(
        roots: [URL],
        minimumBytes: Int64,
        maxFiles: Int
    ) -> [LargeFileItem] {
        let existingRoots = roots.filter { fileManager.fileExists(atPath: $0.path) }
        guard !existingRoots.isEmpty else { return [] }

        let predicate = "kMDItemFSSize >= \(minimumBytes)" as CFString
        guard let query = MDQueryCreate(kCFAllocatorDefault, predicate, nil, nil) else {
            return []
        }

        let scope = existingRoots.map(\.path) as CFArray
        MDQuerySetSearchScope(query, scope, 0)

        // Synchronous execute — works from background threads without a run loop.
        guard MDQueryExecute(query, CFOptionFlags(kMDQuerySynchronous.rawValue)) else {
            return []
        }

        let count = MDQueryGetResultCount(query)
        var results: [LargeFileItem] = []
        var seen = Set<String>()

        for index in 0..<count {
            if results.count >= maxFiles { break }
            guard let raw = MDQueryGetResultAtIndex(query, index) else { continue }
            let item = Unmanaged<MDItem>.fromOpaque(raw).takeUnretainedValue()

            guard let pathCF = MDItemCopyAttribute(item, kMDItemPath) as? String else { continue }
            let standardized = (pathCF as NSString).standardizingPath
            if denylist.isDenied(path: standardized) { continue }
            guard seen.insert(standardized).inserted else { continue }

            let size: Int64
            if let n = MDItemCopyAttribute(item, kMDItemFSSize) as? NSNumber {
                size = n.int64Value
            } else {
                continue
            }
            guard size >= minimumBytes else { continue }

            let modified = MDItemCopyAttribute(item, kMDItemFSContentChangeDate) as? Date
            let ext = (standardized as NSString).pathExtension.lowercased()
            results.append(
                LargeFileItem(
                    path: standardized,
                    byteSize: size,
                    modifiedAt: modified,
                    isVideo: Self.videoExtensions.contains(ext),
                    isImage: Self.imageExtensions.contains(ext),
                    isSelected: false
                )
            )
        }

        return results
    }

    // MARK: - FileManager fallback

    private func fileManagerScan(
        roots: [URL],
        minimumBytes: Int64,
        maxFiles: Int
    ) -> [LargeFileItem] {
        var results: [LargeFileItem] = []
        var seen = Set<String>()

        for root in roots {
            guard fileManager.fileExists(atPath: root.path) else { continue }
            guard let enumerator = fileManager.enumerator(
                at: root,
                includingPropertiesForKeys: [.isRegularFileKey, .fileSizeKey, .contentModificationDateKey, .isPackageKey],
                options: [.skipsHiddenFiles, .skipsPackageDescendants]
            ) else { continue }

            while let url = enumerator.nextObject() as? URL {
                if results.count >= maxFiles { break }
                let path = (url.path as NSString).standardizingPath
                if denylist.isDenied(path: path) { continue }

                guard let values = try? url.resourceValues(forKeys: [
                    .isRegularFileKey, .fileSizeKey, .contentModificationDateKey, .isPackageKey
                ]) else { continue }

                if values.isPackage == true {
                    enumerator.skipDescendants()
                    continue
                }
                guard values.isRegularFile == true else { continue }

                let size = Int64(values.fileSize ?? 0)
                guard size >= minimumBytes else { continue }
                guard seen.insert(path).inserted else { continue }

                let ext = url.pathExtension.lowercased()
                results.append(
                    LargeFileItem(
                        path: path,
                        byteSize: size,
                        modifiedAt: values.contentModificationDate,
                        isVideo: Self.videoExtensions.contains(ext),
                        isImage: Self.imageExtensions.contains(ext),
                        isSelected: false
                    )
                )
            }
        }

        return results
    }

    public static func isVideoPath(_ path: String) -> Bool {
        videoExtensions.contains((path as NSString).pathExtension.lowercased())
    }

    public static func isImagePath(_ path: String) -> Bool {
        imageExtensions.contains((path as NSString).pathExtension.lowercased())
    }
}
