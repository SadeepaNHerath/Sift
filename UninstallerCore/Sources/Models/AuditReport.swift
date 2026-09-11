import Foundation

public enum RemovalOutcome: String, Codable, Sendable {
    case removed
    case retained
    case failed
    case skippedDenied
    case requiresHelper
}

public struct RemovalItemResult: Identifiable, Codable, Sendable {
    public var id: String { path }

    public let path: String
    public let category: ResourceCategory
    public let confidence: ConfidenceLevel
    public let signals: [AttributionSignal]
    public let outcome: RemovalOutcome
    public let errorMessage: String?
    public let byteSize: Int64

    public init(
        path: String,
        category: ResourceCategory,
        confidence: ConfidenceLevel,
        signals: [AttributionSignal],
        outcome: RemovalOutcome,
        errorMessage: String? = nil,
        byteSize: Int64 = 0
    ) {
        self.path = path
        self.category = category
        self.confidence = confidence
        self.signals = signals
        self.outcome = outcome
        self.errorMessage = errorMessage
        self.byteSize = byteSize
    }
}

public struct AuditReport: Identifiable, Codable, Sendable {
    public let id: UUID
    public let appName: String
    public let bundleIdentifier: String
    public let appPath: String
    public let createdAt: Date
    public let items: [RemovalItemResult]
    public let notes: String

    public init(
        id: UUID = UUID(),
        appName: String,
        bundleIdentifier: String,
        appPath: String,
        createdAt: Date = Date(),
        items: [RemovalItemResult],
        notes: String = ""
    ) {
        self.id = id
        self.appName = appName
        self.bundleIdentifier = bundleIdentifier
        self.appPath = appPath
        self.createdAt = createdAt
        self.items = items
        self.notes = notes
    }

    public var removedCount: Int { items.filter { $0.outcome == .removed }.count }
    public var failedCount: Int { items.filter { $0.outcome == .failed }.count }
    public var retainedCount: Int { items.filter { $0.outcome == .retained }.count }

    public var removedBytes: Int64 {
        items.filter { $0.outcome == .removed }.reduce(0) { $0 + $1.byteSize }
    }

    public var humanReadableSummary: String {
        var lines: [String] = []
        lines.append("SafeUninstaller Audit Report")
        lines.append("============================")
        lines.append("App: \(appName) (\(bundleIdentifier))")
        lines.append("Path: \(appPath)")
        lines.append("Date: \(ISO8601DateFormatter().string(from: createdAt))")
        lines.append("Removed: \(removedCount)  Retained: \(retainedCount)  Failed: \(failedCount)")
        lines.append("Freed (approx): \(ByteCountFormatter.string(fromByteCount: removedBytes, countStyle: .file))")
        if !notes.isEmpty {
            lines.append("Notes: \(notes)")
        }
        lines.append("")
        lines.append("Items:")
        for item in items {
            let evidence = item.signals.map(\.rawValue).joined(separator: "+")
            lines.append("- [\(item.outcome.rawValue)] \(item.path) (\(item.confidence.rawValue); \(evidence))"
                + (item.errorMessage.map { " — \($0)" } ?? ""))
        }
        return lines.joined(separator: "\n")
    }
}

/// An orphaned leftover not associated with any currently installed app.
public struct OrphanedResource: Identifiable, Hashable, Codable, Sendable {
    public var id: String { resource.path }

    public let resource: DiscoveredResource
    public let inferredBundleID: String?
    public let inferredName: String?
    public let confidence: ConfidenceLevel
    public var isSelected: Bool

    public init(
        resource: DiscoveredResource,
        inferredBundleID: String? = nil,
        inferredName: String? = nil,
        confidence: ConfidenceLevel = .medium,
        isSelected: Bool = false
    ) {
        self.resource = resource
        self.inferredBundleID = inferredBundleID
        self.inferredName = inferredName
        self.confidence = confidence
        self.isSelected = isSelected
    }
}
