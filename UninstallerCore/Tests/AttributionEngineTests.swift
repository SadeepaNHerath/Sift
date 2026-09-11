import XCTest
@testable import UninstallerCore

final class AttributionEngineTests: XCTestCase {
    var tempRoot: URL!
    var discovery: ResourceDiscoveryEngine!
    var attribution: ResourceAttributionEngine!
    var denylist: SafetyDenylist!

    override func setUpWithError() throws {
        tempRoot = FileManager.default.temporaryDirectory
            .appendingPathComponent("SafeUninstallerTests-\(UUID().uuidString)", isDirectory: true)
        try FileManager.default.createDirectory(at: tempRoot, withIntermediateDirectories: true)
        denylist = SafetyDenylist()
        discovery = ResourceDiscoveryEngine()
        attribution = ResourceAttributionEngine(denylist: denylist)
    }

    override func tearDownWithError() throws {
        try? FileManager.default.removeItem(at: tempRoot)
    }

    private func makeApp(name: String = "DemoApp", bundleID: String = "com.acme.demoapp", team: String? = "ABCD1234") -> AppIdentity {
        AppIdentity(
            path: "/Applications/DemoApp.app",
            bundleIdentifier: bundleID,
            name: name,
            version: "1.0",
            executableName: "DemoApp",
            teamID: team
        )
    }

    private func writeFile(relative: String, contents: String = "x") throws -> URL {
        let url = tempRoot.appendingPathComponent(relative)
        try FileManager.default.createDirectory(at: url.deletingLastPathComponent(), withIntermediateDirectories: true)
        try contents.write(to: url, atomically: true, encoding: .utf8)
        return url
    }

    private func writeDir(relative: String) throws -> URL {
        let url = tempRoot.appendingPathComponent(relative)
        try FileManager.default.createDirectory(at: url, withIntermediateDirectories: true)
        return url
    }

    func testExactBundleIDPreferenceIsHighConfidence() throws {
        let app = makeApp()
        let pref = try writeFile(relative: "Library/Preferences/com.acme.demoapp.plist", contents: "<?xml version=\"1.0\"?><plist><dict></dict></plist>")
        let resource = DiscoveredResource(path: pref.path, category: .preferences, isDirectory: false, byteSize: 10)
        let attributed = attribution.attribute(candidate: resource, to: app)
        XCTAssertNotNil(attributed)
        XCTAssertEqual(attributed?.confidence, .high)
        XCTAssertTrue(attributed?.signals.contains(.exactBundleID) == true)
        XCTAssertTrue(attributed?.isSelected == true)
        XCTAssertFalse(attributed?.isDenied == true)
    }

    func testContainerBundleIDIsHighConfidence() throws {
        let app = makeApp()
        let container = try writeDir(relative: "Library/Containers/com.acme.demoapp")
        let resource = DiscoveredResource(path: container.path, category: .containers, isDirectory: true)
        let attributed = attribution.attribute(candidate: resource, to: app)
        XCTAssertEqual(attributed?.confidence, .high)
        XCTAssertTrue(attributed?.signals.contains(.containerBundleID) == true || attributed?.signals.contains(.exactBundleID) == true)
    }

    func testUnrelatedFileIsNotAttributed() throws {
        let app = makeApp()
        let other = try writeFile(relative: "Library/Preferences/com.other.product.plist")
        let resource = DiscoveredResource(path: other.path, category: .preferences, isDirectory: false)
        let attributed = attribution.attribute(candidate: resource, to: app)
        XCTAssertNil(attributed)
    }

    func testVendorPrefixAloneIsPossiblySharedWhenSiblingExists() throws {
        let app = makeApp()
        let sibling = AppIdentity(
            path: "/Applications/OtherAcme.app",
            bundleIdentifier: "com.acme.other",
            name: "OtherAcme",
            teamID: "ABCD1234"
        )
        let support = try writeDir(relative: "Library/Application Support/com.acme.shared")
        let resource = DiscoveredResource(path: support.path, category: .applicationSupport, isDirectory: true)
        let attributed = attribution.attribute(candidate: resource, to: app, installedApps: [app, sibling])
        XCTAssertNotNil(attributed)
        XCTAssertTrue(attributed?.signals.contains(.vendorPrefix) == true)
        XCTAssertTrue(attributed?.isPossiblyShared == true)
        // Vendor-only should not auto-select as high in typical cases
        XCTAssertNotEqual(attributed?.confidence, .high)
    }

    func testLaunchAgentProgramPathIsHighConfidence() throws {
        let app = makeApp()
        let plist = """
        <?xml version="1.0" encoding="UTF-8"?>
        <!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
        <plist version="1.0"><dict>
          <key>Label</key><string>com.acme.demoapp.helper</string>
          <key>ProgramArguments</key>
          <array><string>/Applications/DemoApp.app/Contents/MacOS/helper</string></array>
        </dict></plist>
        """
        let url = try writeFile(relative: "Library/LaunchAgents/com.acme.demoapp.helper.plist", contents: plist)
        let resource = DiscoveredResource(path: url.path, category: .launchAgents, isDirectory: false)
        let attributed = attribution.attribute(candidate: resource, to: app)
        XCTAssertEqual(attributed?.confidence, .high)
        XCTAssertTrue(attributed?.signals.contains(.launchdProgramPath) == true)
    }

    func testAppleSystemPathIsDenied() {
        let app = makeApp()
        let resource = DiscoveredResource(
            path: "/System/Library/CoreServices/Finder.app",
            category: .other,
            isDirectory: true
        )
        let attributed = attribution.attribute(candidate: resource, to: app)
        XCTAssertEqual(attributed?.isDenied, true)
        XCTAssertEqual(attributed?.isSelected, false)
    }

    func testSharedLibraryRootIsDenied() {
        let home = FileManager.default.homeDirectoryForCurrentUser.path
        let reason = denylist.denyReason(for: home + "/Library/Preferences")
        XCTAssertNotNil(reason)
    }

    func testConfidenceScoreExactBundleDominates() {
        let score = attribution.confidenceScore(signals: [.exactBundleID])
        XCTAssertGreaterThanOrEqual(score, 0.9)
        let level = attribution.confidenceLevel(for: score, signals: [.exactBundleID])
        XCTAssertEqual(level, .high)
    }

    func testNameOnlyMatchIsLowOrMediumAndNotAutoSelectedStrongly() throws {
        let app = makeApp(name: "UniqueZebraTool", bundleID: "com.acme.uniquezebratool")
        // Path contains name but not bundle id
        let folder = try writeDir(relative: "Library/Application Support/UniqueZebraToolData")
        let resource = DiscoveredResource(path: folder.path, category: .applicationSupport, isDirectory: true)
        let attributed = attribution.attribute(candidate: resource, to: app)
        XCTAssertNotNil(attributed)
        XCTAssertTrue(attributed?.signals.contains(.appNameInPath) == true)
        // Name-only should be low-ish and possibly shared
        XCTAssertTrue(attributed?.confidence == .low || attributed?.confidence == .medium)
        XCTAssertEqual(attributed?.isSelected, false)
    }

    func testDiscoveryNeedlesIncludeBundleAndName() {
        let app = makeApp()
        let needles = discovery.searchNeedles(for: app)
        XCTAssertTrue(needles.contains("com.acme.demoapp"))
        XCTAssertTrue(needles.contains { $0.contains("demoapp") })
    }

    func testOrphanInferenceFromPlistName() {
        let engine = OrphanSearchEngine()
        XCTAssertEqual(engine.inferBundleID(from: "com.gone.app.plist"), "com.gone.app")
        XCTAssertEqual(engine.inferBundleID(from: "com.gone.app.savedState"), "com.gone.app")
        XCTAssertNil(engine.inferBundleID(from: "random-folder"))
    }

    func testOrphanSearchFindsMissingAppLeftover() throws {
        let library = try writeDir(relative: "Library/Preferences")
        _ = try writeFile(relative: "Library/Preferences/com.vanished.product.plist")
        _ = try writeFile(relative: "Library/Preferences/com.acme.demoapp.plist")

        let installed = [makeApp()]
        let paths = [
            DiscoveryPath(url: library, category: .preferences)
        ]
        let engine = OrphanSearchEngine(discovery: discovery, denylist: denylist)
        let orphans = engine.findOrphans(installedApps: installed, searchPaths: paths)
        let ids = orphans.compactMap(\.inferredBundleID)
        XCTAssertTrue(ids.contains("com.vanished.product"))
        XCTAssertFalse(ids.contains("com.acme.demoapp"))
    }

    func testRemovalDryRunWritesReport() async throws {
        let app = makeApp()
        let pref = try writeFile(relative: "Library/Preferences/com.acme.demoapp.plist")
        let resource = DiscoveredResource(path: pref.path, category: .preferences, isDirectory: false, byteSize: 4)
        let attributed = AttributedResource(
            resource: resource,
            signals: [.exactBundleID],
            confidence: .high,
            score: 1.0,
            isSelected: true
        )
        let engine = RemovalEngine()
        // Point reports at temp
        let report = await engine.remove(app: app, resources: [attributed], dryRun: true)
        XCTAssertEqual(report.removedCount, 1)
        XCTAssertTrue(FileManager.default.fileExists(atPath: pref.path), "Dry run must not delete")
        XCTAssertTrue(report.notes.lowercased().contains("dry"))
    }
}
