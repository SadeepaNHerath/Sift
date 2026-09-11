import Foundation
import Security
import CoreServices

/// Multi-signal attribution engine that scores whether a resource belongs to an app.
public final class ResourceAttributionEngine: @unchecked Sendable {
    public static let shared = ResourceAttributionEngine()

    private let fileManager: FileManager
    private let denylist: SafetyDenylist
    private let highThreshold: Double
    private let mediumThreshold: Double

    public init(
        fileManager: FileManager = .default,
        denylist: SafetyDenylist = .shared,
        highThreshold: Double = 0.7,
        mediumThreshold: Double = 0.35
    ) {
        self.fileManager = fileManager
        self.denylist = denylist
        self.highThreshold = highThreshold
        self.mediumThreshold = mediumThreshold
    }

    public func attribute(
        candidates: [DiscoveredResource],
        to app: AppIdentity,
        installedApps: [AppIdentity] = []
    ) -> [AttributedResource] {
        candidates.compactMap { candidate in
            attribute(candidate: candidate, to: app, installedApps: installedApps)
        }
        .sorted {
            if $0.confidence != $1.confidence {
                return $0.confidence < $1.confidence
            }
            return $0.score > $1.score
        }
    }

    public func attribute(
        candidate: DiscoveredResource,
        to app: AppIdentity,
        installedApps: [AppIdentity] = []
    ) -> AttributedResource? {
        let path = candidate.path
        let fileName = (path as NSString).lastPathComponent

        if let reason = denylist.denyReason(for: path, bundleHint: app.bundleIdentifier) {
            // Still surface denied items so the UI can explain, but never select them.
            return AttributedResource(
                resource: candidate,
                signals: [],
                confidence: .low,
                score: 0,
                isDenied: true,
                denyReason: reason,
                isPossiblyShared: true,
                isSelected: false
            )
        }

        // Never attribute Apple apps' leftovers via this tool for third-party uninstall of Apple itself.
        if app.isAppleSigned && app.bundleIdentifier.lowercased().hasPrefix("com.apple.") {
            return AttributedResource(
                resource: candidate,
                signals: [],
                confidence: .low,
                score: 0,
                isDenied: true,
                denyReason: "Refusing to uninstall Apple system application resources",
                isPossiblyShared: true,
                isSelected: false
            )
        }

        var signals: [AttributionSignal] = []
        let lowerPath = path.lowercased()
        let lowerName = fileName.lowercased()
        let bundleID = app.bundleIdentifier.lowercased()

        // Exact bundle ID in path/name
        if !bundleID.isEmpty && (lowerName.contains(bundleID) || lowerPath.contains("/\(bundleID)") || lowerPath.contains(bundleID)) {
            signals.append(.exactBundleID)
        }

        // Containers named exactly as bundle ID
        if candidate.category == .containers || candidate.category == .applicationScripts {
            if !bundleID.isEmpty && (lowerName == bundleID || lowerName.hasPrefix(bundleID)) {
                if !signals.contains(.exactBundleID) { signals.append(.exactBundleID) }
                signals.append(.containerBundleID)
            }
        }

        // Saved Application State: com.foo.app.savedState
        if candidate.category == .savedState {
            if !bundleID.isEmpty && lowerName.hasPrefix(bundleID) {
                signals.append(.savedStateName)
                if !signals.contains(.exactBundleID) { signals.append(.exactBundleID) }
            }
        }

        // Vendor prefix (possible shared)
        var possiblyShared = false
        if let vendor = app.vendorPrefix?.lowercased(), !vendor.isEmpty {
            if lowerName.contains(vendor) || lowerPath.contains(vendor) {
                if !signals.contains(.exactBundleID) {
                    signals.append(.vendorPrefix)
                    // Check if other installed apps share this vendor
                    let siblings = installedApps.filter {
                        $0.path != app.path
                            && ($0.vendorPrefix?.lowercased() == vendor
                                || $0.bundleIdentifier.lowercased().hasPrefix(vendor + "."))
                    }
                    if !siblings.isEmpty {
                        possiblyShared = true
                    }
                }
            }
        }

        // App name in path
        let normalizedPath = lowerPath
            .replacingOccurrences(of: " ", with: "")
            .replacingOccurrences(of: "-", with: "")
            .replacingOccurrences(of: "_", with: "")
        if !app.normalizedName.isEmpty && app.normalizedName.count >= 4 {
            if normalizedPath.contains(app.normalizedName) || lowerName.contains(app.name.lowercased()) {
                signals.append(.appNameInPath)
            }
        }

        // LaunchAgent / Daemon Program path points into app bundle
        if candidate.category == .launchAgents || candidate.category == .launchDaemons {
            if launchdPointsToApp(plistPath: path, app: app) {
                signals.append(.launchdProgramPath)
            }
            if plistReferencesBundle(plistPath: path, bundleID: app.bundleIdentifier) {
                signals.append(.plistContentReference)
            }
        }

        // Preference / other plist content
        if path.lowercased().hasSuffix(".plist") {
            if plistReferencesBundle(plistPath: path, bundleID: app.bundleIdentifier) {
                if !signals.contains(.plistContentReference) {
                    signals.append(.plistContentReference)
                }
            }
        }

        // Team ID on helper binaries
        if candidate.category == .privilegedHelpers || !candidate.isDirectory {
            if let team = app.teamID, !team.isEmpty, binaryTeamID(at: path) == team {
                signals.append(.teamIDMatch)
            }
        }

        // Spotlight metadata
        if spotlightBundleID(at: path) == app.bundleIdentifier, !app.bundleIdentifier.isEmpty {
            signals.append(.spotlightBundleID)
        }

        // Application bundle itself
        if candidate.category == .applicationBundle && path == app.path {
            signals.append(.exactBundleID)
        }

        // pkg receipt membership — if path was discovered via receipt category .other with bundle in name
        if candidate.category == .other && !bundleID.isEmpty && lowerPath.contains(bundleID) {
            signals.append(.pkgReceipt)
        }

        guard !signals.isEmpty else { return nil }

        let uniqueSignals = Array(Set(signals))
        let score = confidenceScore(signals: uniqueSignals)
        let level = confidenceLevel(for: score, signals: uniqueSignals)

        // Vendor-only or name-only without stronger signals → possibly shared / lower trust
        if uniqueSignals.allSatisfy({ $0 == .vendorPrefix || $0 == .appNameInPath || $0 == .teamIDMatch }) {
            possiblyShared = true
        }

        return AttributedResource(
            resource: candidate,
            signals: uniqueSignals.sorted { $0.weight > $1.weight },
            confidence: level,
            score: score,
            isDenied: false,
            denyReason: nil,
            isPossiblyShared: possiblyShared
        )
    }

    public func confidenceScore(signals: [AttributionSignal]) -> Double {
        // Soft-OR of weights: 1 - Π(1 - w)
        let product = signals.reduce(1.0) { partial, signal in
            partial * (1.0 - min(signal.weight, 0.99))
        }
        return min(1.0, 1.0 - product)
    }

    public func confidenceLevel(for score: Double, signals: [AttributionSignal]) -> ConfidenceLevel {
        // Exact bundle ID alone is always high.
        if signals.contains(.exactBundleID)
            || signals.contains(.containerBundleID)
            || signals.contains(.launchdProgramPath) {
            return score >= mediumThreshold ? .high : .medium
        }
        if score >= highThreshold { return .high }
        if score >= mediumThreshold { return .medium }
        return .low
    }

    // MARK: - Signal helpers

    private func launchdPointsToApp(plistPath: String, app: AppIdentity) -> Bool {
        guard let dict = NSDictionary(contentsOfFile: plistPath) as? [String: Any] else { return false }
        let appPath = app.path
        if let program = dict["Program"] as? String, program.hasPrefix(appPath) {
            return true
        }
        if let args = dict["ProgramArguments"] as? [String], let first = args.first, first.hasPrefix(appPath) {
            return true
        }
        if let bundleProgram = dict["BundleProgram"] as? String {
            // Relative to app — if Label matches bundle, treat as related when path contains app name.
            if bundleProgram.contains(app.executableName) || plistPath.lowercased().contains(app.bundleIdentifier.lowercased()) {
                return true
            }
        }
        return false
    }

    private func plistReferencesBundle(plistPath: String, bundleID: String) -> Bool {
        guard !bundleID.isEmpty else { return false }
        guard let data = try? Data(contentsOf: URL(fileURLWithPath: plistPath)) else { return false }
        // Binary or XML plist — search string form via PropertyListSerialization
        if let obj = try? PropertyListSerialization.propertyList(from: data, options: [], format: nil) {
            return deepContains(obj, needle: bundleID)
        }
        if let text = String(data: data, encoding: .utf8) {
            return text.contains(bundleID)
        }
        return false
    }

    private func deepContains(_ value: Any, needle: String) -> Bool {
        if let s = value as? String {
            return s.localizedCaseInsensitiveContains(needle)
        }
        if let arr = value as? [Any] {
            return arr.contains { deepContains($0, needle: needle) }
        }
        if let dict = value as? [String: Any] {
            return dict.contains { deepContains($0.key, needle: needle) || deepContains($0.value, needle: needle) }
        }
        return false
    }

    private func binaryTeamID(at path: String) -> String? {
        var isDir: ObjCBool = false
        guard FileManager.default.fileExists(atPath: path, isDirectory: &isDir), !isDir.boolValue else {
            return nil
        }
        var staticCode: SecStaticCode?
        let url = URL(fileURLWithPath: path)
        guard SecStaticCodeCreateWithPath(url as CFURL, [], &staticCode) == errSecSuccess,
              let staticCode else { return nil }
        var info: CFDictionary?
        guard SecCodeCopySigningInformation(staticCode, SecCSFlags(rawValue: kSecCSSigningInformation), &info) == errSecSuccess,
              let dict = info as? [String: Any] else { return nil }
        return dict[kSecCodeInfoTeamIdentifier as String] as? String
    }

    private func spotlightBundleID(at path: String) -> String? {
        let item = MDItemCreate(nil, path as CFString)
        guard let item else { return nil }
        let value = MDItemCopyAttribute(item, kMDItemCFBundleIdentifier)
        return value as? String
    }
}
