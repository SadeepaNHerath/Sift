import Foundation

/// A filesystem resource discovered as a candidate for association with an app.
public struct DiscoveredResource: Identifiable, Hashable, Codable, Sendable {
    public var id: String { path }

    public let path: String
    public let category: ResourceCategory
    public let isDirectory: Bool
    public let byteSize: Int64
    public let isSystemLocation: Bool
    public let requiresPrivilege: Bool

    public init(
        path: String,
        category: ResourceCategory,
        isDirectory: Bool,
        byteSize: Int64 = 0,
        isSystemLocation: Bool = false,
        requiresPrivilege: Bool = false
    ) {
        self.path = path
        self.category = category
        self.isDirectory = isDirectory
        self.byteSize = byteSize
        self.isSystemLocation = isSystemLocation
        self.requiresPrivilege = requiresPrivilege
    }

    public var url: URL { URL(fileURLWithPath: path) }

    public var displayName: String { (path as NSString).lastPathComponent }

    public var formattedSize: String {
        ByteCountFormatter.string(fromByteCount: byteSize, countStyle: .file)
    }
}

/// A discovered resource with attribution evidence and confidence.
public struct AttributedResource: Identifiable, Hashable, Codable, Sendable {
    public var id: String { resource.path }

    public let resource: DiscoveredResource
    public let signals: [AttributionSignal]
    public let confidence: ConfidenceLevel
    public let score: Double
    public let isDenied: Bool
    public let denyReason: String?
    public let isPossiblyShared: Bool
    public var isSelected: Bool

    public init(
        resource: DiscoveredResource,
        signals: [AttributionSignal],
        confidence: ConfidenceLevel,
        score: Double,
        isDenied: Bool = false,
        denyReason: String? = nil,
        isPossiblyShared: Bool = false,
        isSelected: Bool? = nil
    ) {
        self.resource = resource
        self.signals = signals
        self.confidence = confidence
        self.score = score
        self.isDenied = isDenied
        self.denyReason = denyReason
        self.isPossiblyShared = isPossiblyShared
        if let isSelected {
            self.isSelected = isSelected
        } else if isDenied {
            self.isSelected = false
        } else {
            // High confidence pre-selected; Medium/Low require user review.
            self.isSelected = confidence == .high
        }
    }

    public var evidenceSummary: String {
        signals.map(\.displayName).joined(separator: ", ")
    }
}

/// Result of scanning related files for one application.
public struct AttributionResult: Sendable {
    public let app: AppIdentity
    public let resources: [AttributedResource]
    public let scannedAt: Date

    public init(app: AppIdentity, resources: [AttributedResource], scannedAt: Date = Date()) {
        self.app = app
        self.resources = resources
        self.scannedAt = scannedAt
    }

    public var selectedResources: [AttributedResource] {
        resources.filter(\.isSelected)
    }

    public var totalSelectedBytes: Int64 {
        selectedResources.reduce(0) { $0 + $1.resource.byteSize }
    }

    public func resources(with confidence: ConfidenceLevel) -> [AttributedResource] {
        resources.filter { $0.confidence == confidence && !$0.isDenied }
    }
}
