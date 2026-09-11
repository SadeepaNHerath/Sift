import Foundation

/// Evidence that a discovered resource belongs to an application.
public enum AttributionSignal: String, Codable, Hashable, Sendable, CaseIterable {
    case exactBundleID
    case vendorPrefix
    case appNameInPath
    case launchdProgramPath
    case plistContentReference
    case teamIDMatch
    case pkgReceipt
    case spotlightBundleID
    case containerBundleID
    case savedStateName

    public var displayName: String {
        switch self {
        case .exactBundleID: return "Exact Bundle ID"
        case .vendorPrefix: return "Vendor Prefix"
        case .appNameInPath: return "App Name in Path"
        case .launchdProgramPath: return "Launchd Program Path"
        case .plistContentReference: return "Plist Content Reference"
        case .teamIDMatch: return "Code Signature Team ID"
        case .pkgReceipt: return "Installer Receipt"
        case .spotlightBundleID: return "Spotlight Metadata"
        case .containerBundleID: return "App Container"
        case .savedStateName: return "Saved Application State"
        }
    }

    /// Relative weight used by the confidence scorer (0...1 contribution scale via sum).
    public var weight: Double {
        switch self {
        case .exactBundleID: return 1.0
        case .containerBundleID: return 0.95
        case .launchdProgramPath: return 0.9
        case .pkgReceipt: return 0.85
        case .spotlightBundleID: return 0.8
        case .plistContentReference: return 0.75
        case .savedStateName: return 0.7
        case .teamIDMatch: return 0.55
        case .vendorPrefix: return 0.4
        case .appNameInPath: return 0.25
        }
    }
}

public enum ConfidenceLevel: String, Codable, Hashable, Sendable, Comparable {
    case high
    case medium
    case low

    public var displayName: String {
        switch self {
        case .high: return "High"
        case .medium: return "Medium"
        case .low: return "Low"
        }
    }

    public var sortOrder: Int {
        switch self {
        case .high: return 0
        case .medium: return 1
        case .low: return 2
        }
    }

    public static func < (lhs: ConfidenceLevel, rhs: ConfidenceLevel) -> Bool {
        lhs.sortOrder < rhs.sortOrder
    }
}

public enum ResourceCategory: String, Codable, Hashable, Sendable, CaseIterable {
    case applicationBundle
    case preferences
    case caches
    case applicationSupport
    case containers
    case groupContainers
    case savedState
    case logs
    case applicationScripts
    case webKit
    case httpStorages
    case launchAgents
    case launchDaemons
    case privilegedHelpers
    case crashReports
    case other

    public var displayName: String {
        switch self {
        case .applicationBundle: return "Application"
        case .preferences: return "Preferences"
        case .caches: return "Caches"
        case .applicationSupport: return "Application Support"
        case .containers: return "Containers"
        case .groupContainers: return "Group Containers"
        case .savedState: return "Saved State"
        case .logs: return "Logs"
        case .applicationScripts: return "Application Scripts"
        case .webKit: return "WebKit Data"
        case .httpStorages: return "HTTP Storages"
        case .launchAgents: return "Launch Agents"
        case .launchDaemons: return "Launch Daemons"
        case .privilegedHelpers: return "Privileged Helpers"
        case .crashReports: return "Crash Reports"
        case .other: return "Other"
        }
    }
}
