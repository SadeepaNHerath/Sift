import Foundation

public enum StorageCategoryKind: String, Codable, Sendable, CaseIterable, Identifiable {
    case applications
    case caches
    case downloads
    case movies
    case developer
    case trash

    public var id: String { rawValue }

    public var displayName: String {
        switch self {
        case .applications: return "Applications"
        case .caches: return "Caches"
        case .downloads: return "Downloads"
        case .movies: return "Movies"
        case .developer: return "Developer"
        case .trash: return "Trash"
        }
    }

    public var systemImage: String {
        switch self {
        case .applications: return "square.grid.2x2"
        case .caches: return "internaldrive"
        case .downloads: return "arrow.down.circle"
        case .movies: return "film"
        case .developer: return "hammer"
        case .trash: return "trash"
        }
    }
}

public struct StorageCategorySize: Identifiable, Sendable {
    public var id: String { kind.rawValue }
    public let kind: StorageCategoryKind
    public let byteSize: Int64
    /// True when directory sizing hit the sample cap (Overview only).
    public let isApproximate: Bool

    public init(kind: StorageCategoryKind, byteSize: Int64, isApproximate: Bool = false) {
        self.kind = kind
        self.byteSize = byteSize
        self.isApproximate = isApproximate
    }

    public var formattedSize: String {
        let raw = ByteCountFormatter.string(fromByteCount: byteSize, countStyle: .file)
        return isApproximate ? "~\(raw)" : raw
    }
}

public struct StorageOverview: Sendable {
    public let totalBytes: Int64
    public let availableBytes: Int64
    public let categories: [StorageCategorySize]
    public let scannedAt: Date

    public init(
        totalBytes: Int64,
        availableBytes: Int64,
        categories: [StorageCategorySize],
        scannedAt: Date = Date()
    ) {
        self.totalBytes = totalBytes
        self.availableBytes = availableBytes
        self.categories = categories
        self.scannedAt = scannedAt
    }

    public var usedBytes: Int64 { max(0, totalBytes - availableBytes) }

    public var usedFraction: Double {
        guard totalBytes > 0 else { return 0 }
        return min(1, Double(usedBytes) / Double(totalBytes))
    }

    public var formattedAvailable: String {
        ByteCountFormatter.string(fromByteCount: availableBytes, countStyle: .file)
    }

    public var formattedTotal: String {
        ByteCountFormatter.string(fromByteCount: totalBytes, countStyle: .file)
    }
}

public struct LargeFileItem: Identifiable, Hashable, Sendable {
    public var id: String { path }
    public let path: String
    public let byteSize: Int64
    public let modifiedAt: Date?
    public let isVideo: Bool
    public let isImage: Bool
    public var isSelected: Bool

    public init(
        path: String,
        byteSize: Int64,
        modifiedAt: Date? = nil,
        isVideo: Bool,
        isImage: Bool = false,
        isSelected: Bool = false
    ) {
        self.path = path
        self.byteSize = byteSize
        self.modifiedAt = modifiedAt
        self.isVideo = isVideo
        self.isImage = isImage
        self.isSelected = isSelected
    }

    public var displayName: String { (path as NSString).lastPathComponent }
    public var formattedSize: String {
        ByteCountFormatter.string(fromByteCount: byteSize, countStyle: .file)
    }
    public var fileExtension: String {
        (path as NSString).pathExtension.lowercased()
    }

    public var isCompressible: Bool { isVideo || isImage }
}

public enum VideoQualityPreset: String, Codable, Sendable, CaseIterable, Identifiable {
    case smaller
    case balanced
    case higherQuality

    public var id: String { rawValue }

    public var displayName: String {
        switch self {
        case .smaller: return "Smaller"
        case .balanced: return "Balanced"
        case .higherQuality: return "Higher quality"
        }
    }

    public var detail: String {
        switch self {
        case .smaller: return "1080p HEVC / medium — max savings"
        case .balanced: return "HEVC high quality — good size/quality"
        case .higherQuality: return "HEVC highest / 4K when source allows"
        }
    }

    /// Rough fraction of source bitrate used for pre-compress estimates.
    public var estimateScale: Double {
        switch self {
        case .smaller: return 0.45
        case .balanced: return 0.70
        case .higherQuality: return 0.90
        }
    }
}

public enum ImageOutputFormat: String, Codable, Sendable, CaseIterable, Identifiable {
    case automatic
    case heic
    case jpeg

    public var id: String { rawValue }

    public var displayName: String {
        switch self {
        case .automatic: return "Automatic"
        case .heic: return "HEIC"
        case .jpeg: return "JPEG"
        }
    }

    public var detail: String {
        switch self {
        case .automatic: return "Pick whichever of HEIC or JPEG is smaller"
        case .heic: return "Force HEIC when it is smaller than the original"
        case .jpeg: return "Force JPEG (~0.7 quality) when smaller"
        }
    }
}

public struct CacheItem: Identifiable, Hashable, Sendable {
    public var id: String { path }
    public let path: String
    public let byteSize: Int64
    public let inferredOwner: String?
    public let note: String
    public var isSelected: Bool

    public init(
        path: String,
        byteSize: Int64,
        inferredOwner: String? = nil,
        note: String = "",
        isSelected: Bool = false
    ) {
        self.path = path
        self.byteSize = byteSize
        self.inferredOwner = inferredOwner
        self.note = note
        self.isSelected = isSelected
    }

    public var displayName: String { (path as NSString).lastPathComponent }
    public var formattedSize: String {
        ByteCountFormatter.string(fromByteCount: byteSize, countStyle: .file)
    }
}

public enum DevDependencyGroup: String, Codable, Sendable, CaseIterable, Identifiable {
    case xcode
    case packageCaches
    case projectFolders

    public var id: String { rawValue }

    public var displayName: String {
        switch self {
        case .xcode: return "Xcode"
        case .packageCaches: return "Package Caches"
        case .projectFolders: return "Project Dependencies"
        }
    }
}

public struct DevDependencyItem: Identifiable, Hashable, Sendable {
    public var id: String { path }
    public let path: String
    public let byteSize: Int64
    public let group: DevDependencyGroup
    public let consequence: String
    public let isHighRisk: Bool
    public var isSelected: Bool

    public init(
        path: String,
        byteSize: Int64,
        group: DevDependencyGroup,
        consequence: String,
        isHighRisk: Bool = false,
        isSelected: Bool = false
    ) {
        self.path = path
        self.byteSize = byteSize
        self.group = group
        self.consequence = consequence
        self.isHighRisk = isHighRisk
        self.isSelected = isSelected
    }

    public var displayName: String { (path as NSString).lastPathComponent }
    public var formattedSize: String {
        ByteCountFormatter.string(fromByteCount: byteSize, countStyle: .file)
    }
}

public struct VideoCompressionResult: Sendable {
    public let originalPath: String
    public let outputPath: String
    public let originalBytes: Int64
    public let compressedBytes: Int64
    public let success: Bool
    public let errorMessage: String?
    public let wasCancelled: Bool

    public init(
        originalPath: String,
        outputPath: String,
        originalBytes: Int64,
        compressedBytes: Int64,
        success: Bool,
        errorMessage: String? = nil,
        wasCancelled: Bool = false
    ) {
        self.originalPath = originalPath
        self.outputPath = outputPath
        self.originalBytes = originalBytes
        self.compressedBytes = compressedBytes
        self.success = success
        self.errorMessage = errorMessage
        self.wasCancelled = wasCancelled
    }

    public var savedBytes: Int64 { max(0, originalBytes - compressedBytes) }

    public var savedPercent: Int {
        guard originalBytes > 0 else { return 0 }
        return Int((Double(savedBytes) / Double(originalBytes)) * 100.0)
    }
}

public struct CLICleanupAction: Identifiable, Sendable, Hashable {
    public var id: String { toolID }
    public let toolID: String
    public let title: String
    public let detail: String
    public let isAvailable: Bool

    public init(toolID: String, title: String, detail: String, isAvailable: Bool) {
        self.toolID = toolID
        self.title = title
        self.detail = detail
        self.isAvailable = isAvailable
    }
}

public struct CLICleanupResult: Sendable {
    public let toolID: String
    public let success: Bool
    public let summary: String

    public init(toolID: String, success: Bool, summary: String) {
        self.toolID = toolID
        self.success = success
        self.summary = summary
    }
}
