import Foundation

/// Finds large files under common user media folders.
public final class LargeFileScanner: @unchecked Sendable {
    public static let shared = LargeFileScanner()

    public static let videoExtensions: Set<String> = ["mov", "mp4", "m4v", "avi", "mkv", "mpg", "mpeg", "wmv"]

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
        maxFiles: Int = 5_000
    ) -> [LargeFileItem] {
        let searchRoots = roots ?? defaultRoots()
        var results: [LargeFileItem] = []
        var seen = Set<String>()

        for root in searchRoots {
            guard fileManager.fileExists(atPath: root.path) else { continue }
            guard let enumerator = fileManager.enumerator(
                at: root,
                includingPropertiesForKeys: [.isRegularFileKey, .fileSizeKey, .contentModificationDateKey, .isPackageKey],
                options: [.skipsHiddenFiles, .skipsPackageDescendants]
            ) else { continue }

            while let url = enumerator.nextObject() as? URL {
                if results.count >= maxFiles { break }
                let path = url.path
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
                        isSelected: false
                    )
                )
            }
        }

        return results.sorted { $0.byteSize > $1.byteSize }
    }

    public static func isVideoPath(_ path: String) -> Bool {
        videoExtensions.contains((path as NSString).pathExtension.lowercased())
    }
}
