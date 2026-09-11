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

    public init(kind: StorageCategoryKind, byteSize: Int64) {
        self.kind = kind
        self.byteSize = byteSize
    }

    public var formattedSize: String {
        ByteCountFormatter.string(fromByteCount: byteSize, countStyle: .file)
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
    public var isSelected: Bool

    public init(
        path: String,
        byteSize: Int64,
        modifiedAt: Date? = nil,
        isVideo: Bool,
        isSelected: Bool = false
    ) {
        self.path = path
        self.byteSize = byteSize
        self.modifiedAt = modifiedAt
        self.isVideo = isVideo
        self.isSelected = isSelected
    }

    public var displayName: String { (path as NSString).lastPathComponent }
    public var formattedSize: String {
        ByteCountFormatter.string(fromByteCount: byteSize, countStyle: .file)
    }
    public var fileExtension: String {
        (path as NSString).pathExtension.lowercased()
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

    public init(
        originalPath: String,
        outputPath: String,
        originalBytes: Int64,
        compressedBytes: Int64,
        success: Bool,
        errorMessage: String? = nil
    ) {
        self.originalPath = originalPath
        self.outputPath = outputPath
        self.originalBytes = originalBytes
        self.compressedBytes = compressedBytes
        self.success = success
        self.errorMessage = errorMessage
    }

    public var savedBytes: Int64 { max(0, originalBytes - compressedBytes) }
}
