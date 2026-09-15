import XCTest
@testable import UninstallerCore

final class SecurityAllowlistTests: XCTestCase {
    func testBlocksUsrLocal() {
        XCTAssertNotNil(PathAllowlist.privilegedTrashDenialReason(for: "/usr/local"))
        XCTAssertNotNil(PathAllowlist.privilegedTrashDenialReason(for: "/usr/local/bin/foo"))
        XCTAssertNotNil(SafetyDenylist.shared.denyReason(for: "/usr/local/Cellar/wget"))
    }

    func testBlocksKeychainsAndExtensions() {
        XCTAssertNotNil(PathAllowlist.privilegedTrashDenialReason(for: "/Library/Keychains/System.keychain"))
        XCTAssertNotNil(PathAllowlist.privilegedTrashDenialReason(for: "/Library/Extensions/Foo.kext"))
        XCTAssertNotNil(PathAllowlist.privilegedTrashDenialReason(for: "/Library/Frameworks/Foo.framework"))
        XCTAssertNotNil(PathAllowlist.privilegedTrashDenialReason(for: "/Library/Preferences/SystemConfiguration"))
    }

    func testAllowsSingleLeafLaunchDaemon() {
        XCTAssertNil(PathAllowlist.privilegedTrashDenialReason(for: "/Library/LaunchDaemons/com.acme.helper.plist"))
        XCTAssertNil(PathAllowlist.privilegedTrashDenialReason(for: "/Library/PrivilegedHelperTools/com.acme.helper"))
    }

    func testRefusesSharedLibraryRoots() {
        XCTAssertNotNil(PathAllowlist.privilegedTrashDenialReason(for: "/Library/LaunchDaemons"))
        XCTAssertNotNil(PathAllowlist.privilegedTrashDenialReason(for: "/Library/Application Support"))
        XCTAssertNotNil(SafetyDenylist.shared.denyReason(for: "/Library/Preferences"))
    }

    func testRejectsNestedLaunchAgentPaths() {
        let reason = PathAllowlist.privilegedTrashDenialReason(for: "/Library/LaunchAgents/nested/com.acme.plist")
        XCTAssertNotNil(reason)
    }

    func testLaunchdLabelValidation() {
        XCTAssertTrue(PathAllowlist.isValidLaunchdLabel("com.acme.helper"))
        XCTAssertFalse(PathAllowlist.isValidLaunchdLabel("com.apple.WindowServer"))
        XCTAssertFalse(PathAllowlist.isValidLaunchdLabel("../etc/passwd"))
        XCTAssertFalse(PathAllowlist.isValidLaunchdLabel("com.acme/helper"))
        XCTAssertFalse(PathAllowlist.isValidLaunchdLabel(""))
    }

    func testLaunchdDomainValidation() {
        XCTAssertTrue(PathAllowlist.isValidLaunchdDomain("system", peerUID: 501))
        XCTAssertTrue(PathAllowlist.isValidLaunchdDomain("gui/501", peerUID: 501))
        XCTAssertFalse(PathAllowlist.isValidLaunchdDomain("gui/502", peerUID: 501))
        XCTAssertFalse(PathAllowlist.isValidLaunchdDomain("user/501", peerUID: 501))
    }

    func testBundleIDBoundaryMatch() {
        XCTAssertTrue(PathAllowlist.pathMatchesBundleID(
            "/Users/x/Library/Preferences/com.acme.app.plist",
            bundleID: "com.acme.app"
        ))
        XCTAssertFalse(PathAllowlist.pathMatchesBundleID(
            "/Users/x/Library/Preferences/com.acme.appExtra.plist",
            bundleID: "com.acme.app"
        ))
        XCTAssertFalse(PathAllowlist.pathMatchesBundleID(
            "/tmp/foocom.acme.appbar",
            bundleID: "com.acme.app"
        ))
    }

    func testProgramPathRequiresBoundary() {
        XCTAssertTrue(PathAllowlist.programPointsIntoApp(
            program: "/Applications/Foo.app/Contents/MacOS/helper",
            appPath: "/Applications/Foo.app"
        ))
        XCTAssertFalse(PathAllowlist.programPointsIntoApp(
            program: "/Applications/Foo.appExtra/Contents/MacOS/helper",
            appPath: "/Applications/Foo.app"
        ))
    }

    func testVendorPrefixRejectsBareCom() {
        let app = AppIdentity(path: "/Applications/X.app", bundleIdentifier: "com.X", name: "X")
        XCTAssertNil(app.vendorPrefix)
        let good = AppIdentity(path: "/Applications/Y.app", bundleIdentifier: "com.acme.y", name: "Y")
        XCTAssertEqual(good.vendorPrefix, "com.acme")
    }

    func testOrphanSkipsInstalledSiblings() {
        let engine = OrphanSearchEngine()
        let installed: Set<String> = ["com.acme.app"]
        XCTAssertTrue(engine.isRelated(to: "com.acme.app.helper", installedIDs: installed))
        XCTAssertTrue(engine.isRelated(to: "com.acme.app", installedIDs: installed))
        XCTAssertFalse(engine.isRelated(to: "com.other.product", installedIDs: installed))
    }

    func testOrphanHidesWhatsAppFamilyWhenInstalled() {
        let engine = OrphanSearchEngine()
        let installedIDs: Set<String> = ["net.whatsapp.WhatsApp"]
        let vendors: Set<String> = ["net.whatsapp"]
        XCTAssertTrue(engine.shouldSkipInferredID(
            "group.net.whatsapp.WhatsApp.private",
            installedIDs: installedIDs,
            installedVendors: vendors
        ))
        XCTAssertTrue(engine.shouldSkipInferredID(
            "group.net.whatsapp.WhatsApp.shared",
            installedIDs: installedIDs,
            installedVendors: vendors
        ))
        XCTAssertTrue(engine.shouldSkipInferredID(
            "group.com.facebook.family",
            installedIDs: installedIDs,
            installedVendors: vendors
        ))
        XCTAssertFalse(engine.shouldSkipInferredID(
            "com.gone.app",
            installedIDs: installedIDs,
            installedVendors: vendors
        ))
    }

    func testOrphanHidesDockerAndCUPS() {
        let engine = OrphanSearchEngine()
        let installedIDs: Set<String> = ["com.docker.docker"]
        let vendors: Set<String> = ["com.docker"]
        XCTAssertTrue(engine.shouldSkipInferredID(
            "com.docker.vmnetd",
            installedIDs: installedIDs,
            installedVendors: vendors
        ))
        XCTAssertTrue(engine.shouldSkipInferredID(
            "org.cups.PrintingPrefs.plist",
            installedIDs: [],
            installedVendors: []
        ))
        XCTAssertTrue(engine.shouldSkipInferredID(
            "is.workflow.shortcuts",
            installedIDs: [],
            installedVendors: []
        ))
    }

    func testNormalizeStripsGroupPrefix() {
        XCTAssertEqual(
            OrphanSearchEngine.normalizeBundleID("group.net.whatsapp.WhatsApp.private"),
            "net.whatsapp.whatsapp.private"
        )
        XCTAssertEqual(OrphanSearchEngine.vendorPrefix(from: "net.whatsapp.WhatsApp"), "net.whatsapp")
    }

    func testSubstringFalsePositiveNotExactBundle() throws {
        let temp = FileManager.default.temporaryDirectory.appendingPathComponent(UUID().uuidString)
        try FileManager.default.createDirectory(at: temp, withIntermediateDirectories: true)
        defer { try? FileManager.default.removeItem(at: temp) }

        let decoy = temp.appendingPathComponent("com.acme.appExtra.plist")
        try "x".write(to: decoy, atomically: true, encoding: .utf8)

        let app = AppIdentity(
            path: "/Applications/Demo.app",
            bundleIdentifier: "com.acme.app",
            name: "Demo"
        )
        let resource = DiscoveredResource(path: decoy.path, category: .preferences, isDirectory: false)
        let attributed = ResourceAttributionEngine().attribute(candidate: resource, to: app)
        // May match vendor prefix, but must never be High / exactBundleID / auto-selected
        if let attributed {
            XCTAssertFalse(attributed.signals.contains(.exactBundleID))
            XCTAssertNotEqual(attributed.confidence, .high)
            XCTAssertFalse(attributed.isSelected)
        }

        // True substring with no path-component boundary and no vendor leaf match
        let unrelated = temp.appendingPathComponent("foocom.acme.appbar.plist")
        try "x".write(to: unrelated, atomically: true, encoding: .utf8)
        let unrelatedResource = DiscoveredResource(path: unrelated.path, category: .preferences, isDirectory: false)
        let unrelatedAttr = ResourceAttributionEngine().attribute(candidate: unrelatedResource, to: app)
        XCTAssertNil(unrelatedAttr)
    }

    func testDeniesSensitiveHomePaths() {
        let home = FileManager.default.homeDirectoryForCurrentUser.path
        XCTAssertNotNil(SafetyDenylist.shared.denyReason(for: home))
        XCTAssertNotNil(SafetyDenylist.shared.denyReason(for: home + "/Library/Keychains/login.keychain-db"))
        XCTAssertNotNil(SafetyDenylist.shared.denyReason(for: home + "/Library/Mail/V10"))
        XCTAssertNotNil(SafetyDenylist.shared.denyReason(for: home + "/Library/Messages"))
        XCTAssertNotNil(SafetyDenylist.shared.denyReason(for: home + "/Library/Safari/Bookmarks.plist"))
        XCTAssertNotNil(SafetyDenylist.shared.denyReason(for: home + "/.ssh/id_ed25519"))
        XCTAssertNotNil(SafetyDenylist.shared.denyReason(for: home + "/.gnupg/private-keys-v1.d"))
        XCTAssertNotNil(SafetyDenylist.shared.preTrashDenialReason(for: home + "/Library/Accounts"))
    }

    func testApplicationSupportSingleLeafOnly() {
        XCTAssertNil(PathAllowlist.privilegedTrashDenialReason(for: "/Library/Application Support/Acme"))
        XCTAssertNotNil(PathAllowlist.privilegedTrashDenialReason(for: "/Library/Application Support/Acme/deep/file"))
    }

    func testShortNeedleBoundaryMatching() {
        XCTAssertTrue(ResourceDiscoveryEngine.pathMatchesNeedle(
            nameLower: "box",
            pathLower: "/users/x/library/application support/box",
            needle: "box"
        ))
        XCTAssertFalse(ResourceDiscoveryEngine.pathMatchesNeedle(
            nameLower: "sandbox",
            pathLower: "/users/x/library/application support/sandbox",
            needle: "box"
        ))
        XCTAssertTrue(ResourceDiscoveryEngine.pathMatchesNeedle(
            nameLower: "longapplicationname",
            pathLower: "/tmp/longapplicationname",
            needle: "longapplicationname"
        ))
    }
}
