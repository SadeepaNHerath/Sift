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

        if app.bundleIdentifier.lowercased().hasPrefix("com.apple.") {
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
        let lowerName = fileName.lowercased()
        let bundleID = app.bundleIdentifier.lowercased()

        // Exact bundle ID — path-component / filename boundary match only
        if !bundleID.isEmpty && PathAllowlist.pathMatchesBundleID(path, bundleID: bundleID) {
            signals.append(.exactBundleID)
        }

        if candidate.category == .containers || candidate.category == .applicationScripts {
            if !bundleID.isEmpty && (lowerName == bundleID || lowerName.hasPrefix(bundleID + ".")) {
                if !signals.contains(.exactBundleID) { signals.append(.exactBundleID) }
                signals.append(.containerBundleID)
            }
        }

        if candidate.category == .savedState {
            if !bundleID.isEmpty && (lowerName == bundleID + ".savedstate" || lowerName.hasPrefix(bundleID + ".")) {
                signals.append(.savedStateName)
                if !signals.contains(.exactBundleID) { signals.append(.exactBundleID) }
            }
        }

        var possiblyShared = false
        if let vendor = app.vendorPrefix?.lowercased(), !vendor.isEmpty {
            if PathAllowlist.pathMatchesBundleID(path, bundleID: vendor)
                || lowerName.hasPrefix(vendor + ".")
                || lowerName == vendor {
                if !signals.contains(.exactBundleID) {
                    signals.append(.vendorPrefix)
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

        let lowerPath = path.lowercased()
        let normalizedPath = lowerPath
            .replacingOccurrences(of: " ", with: "")
            .replacingOccurrences(of: "-", with: "")
            .replacingOccurrences(of: "_", with: "")
        if !app.normalizedName.isEmpty && app.normalizedName.count >= 4 {
            if normalizedPath.contains(app.normalizedName) || lowerName.contains(app.name.lowercased()) {
                signals.append(.appNameInPath)
            }
        }

        if candidate.category == .launchAgents || candidate.category == .launchDaemons {
            if launchdPointsToApp(plistPath: path, app: app) {
                signals.append(.launchdProgramPath)
            }
            if plistReferencesBundle(plistPath: path, bundleID: app.bundleIdentifier) {
                signals.append(.plistContentReference)
            }
        }

        if path.lowercased().hasSuffix(".plist") {
            if plistReferencesBundle(plistPath: path, bundleID: app.bundleIdentifier) {
                if !signals.contains(.plistContentReference) {
                    signals.append(.plistContentReference)
                }
            }
        }

        if candidate.category == .privilegedHelpers || !candidate.isDirectory {
            if let team = app.teamID, !team.isEmpty, binaryTeamID(at: path) == team {
                signals.append(.teamIDMatch)
            }
        }

        if spotlightBundleID(at: path) == app.bundleIdentifier, !app.bundleIdentifier.isEmpty {
            signals.append(.spotlightBundleID)
        }

        if candidate.category == .applicationBundle && path == app.path {
            signals.append(.exactBundleID)
        }

        if candidate.category == .other && !bundleID.isEmpty && PathAllowlist.pathMatchesBundleID(path, bundleID: bundleID) {
            signals.append(.pkgReceipt)
        }

        guard !signals.isEmpty else { return nil }

        let uniqueSignals = Array(Set(signals))
        var score = confidenceScore(signals: uniqueSignals)
        var level = confidenceLevel(for: score, signals: uniqueSignals)

        // Never promote name-only / vendor-only / team-only to High
        let weakOnly = uniqueSignals.allSatisfy {
            $0 == .vendorPrefix || $0 == .appNameInPath || $0 == .teamIDMatch
        }
        if weakOnly {
            possiblyShared = true
            level = score >= mediumThreshold ? .medium : .low
            if level == .high { level = .medium }
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
        let product = signals.reduce(1.0) { partial, signal in
            partial * (1.0 - min(signal.weight, 0.99))
        }
        return min(1.0, 1.0 - product)
    }

    public func confidenceLevel(for score: Double, signals: [AttributionSignal]) -> ConfidenceLevel {
        let strong = signals.contains(.exactBundleID)
            || signals.contains(.containerBundleID)
            || signals.contains(.launchdProgramPath)
        if strong {
            return .high
        }
        // Name/vendor alone never High
        let weakOnly = signals.allSatisfy {
            $0 == .vendorPrefix || $0 == .appNameInPath || $0 == .teamIDMatch
        }
        if weakOnly {
            return score >= mediumThreshold ? .medium : .low
        }
        if score >= highThreshold { return .high }
        if score >= mediumThreshold { return .medium }
        return .low
    }

    private func launchdPointsToApp(plistPath: String, app: AppIdentity) -> Bool {
        guard let dict = NSDictionary(contentsOfFile: plistPath) as? [String: Any] else { return false }
        if let program = dict["Program"] as? String,
           PathAllowlist.programPointsIntoApp(program: program, appPath: app.path) {
            return true
        }
        if let args = dict["ProgramArguments"] as? [String], let first = args.first,
           PathAllowlist.programPointsIntoApp(program: first, appPath: app.path) {
            return true
        }
        if let bundleProgram = dict["BundleProgram"] as? String {
            if !app.executableName.isEmpty && bundleProgram.contains(app.executableName) {
                return PathAllowlist.pathMatchesBundleID(plistPath, bundleID: app.bundleIdentifier)
            }
        }
        return false
    }

    private func plistReferencesBundle(plistPath: String, bundleID: String) -> Bool {
        guard !bundleID.isEmpty else { return false }
        guard let data = try? Data(contentsOf: URL(fileURLWithPath: plistPath)) else { return false }
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
