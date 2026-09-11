import XCTest
@testable import UninstallerCore

final class StorageToolkitTests: XCTestCase {
    var tempRoot: URL!
    var permissiveDenylist: SafetyDenylist!

    override func setUpWithError() throws {
        tempRoot = FileManager.default.temporaryDirectory
            .appendingPathComponent("StorageToolkitTests-\(UUID().uuidString)", isDirectory: true)
        try FileManager.default.createDirectory(at: tempRoot, withIntermediateDirectories: true)
        // Avoid production denylist interactions with /var/folders fixtures.
        permissiveDenylist = SafetyDenylist(
            protectedPrefixes: ["/System", "/usr/bin"],
            protectedBundlePrefixes: ["com.apple."]
        )
    }

    override func tearDownWithError() throws {
        try? FileManager.default.removeItem(at: tempRoot)
    }

    private func writeFile(at url: URL, size: Int) throws {
        try FileManager.default.createDirectory(at: url.deletingLastPathComponent(), withIntermediateDirectories: true)
        let data = Data(repeating: 0x41, count: size)
        try data.write(to: url)
    }

    private func standardized(_ path: String) -> String {
        (path as NSString).standardizingPath
    }

    func testLargeFileScannerRespectsThresholdAndVideoFlag() throws {
        let downloads = tempRoot.appendingPathComponent("Downloads")
        try FileManager.default.createDirectory(at: downloads, withIntermediateDirectories: true)

        let small = downloads.appendingPathComponent("small.txt")
        let big = downloads.appendingPathComponent("movie.mp4")
        let bigDoc = downloads.appendingPathComponent("archive.zip")
        try writeFile(at: small, size: 1024)
        try writeFile(at: big, size: 200_000)
        try writeFile(at: bigDoc, size: 200_000)

        let scanner = LargeFileScanner(denylist: permissiveDenylist)
        let results = scanner.scan(roots: [downloads], minimumBytes: 100_000, maxFiles: 100)
        XCTAssertEqual(results.count, 2, "Expected 2 large files, got \(results.map(\.displayName))")
        XCTAssertTrue(results.contains { standardized($0.path) == standardized(big.path) && $0.isVideo })
        XCTAssertTrue(results.contains { standardized($0.path) == standardized(bigDoc.path) && !$0.isVideo })
        XCTAssertFalse(results.contains { standardized($0.path) == standardized(small.path) })
    }

    func testCacheCleanerSkipsAppleLeaves() throws {
        let home = try XCTUnwrap(tempRoot)
        let caches = home.appendingPathComponent("Library/Caches")
        try FileManager.default.createDirectory(at: caches.appendingPathComponent("com.apple.Safari"), withIntermediateDirectories: true)
        try FileManager.default.createDirectory(at: caches.appendingPathComponent("com.acme.demo"), withIntermediateDirectories: true)
        try writeFile(at: caches.appendingPathComponent("com.acme.demo/data.bin"), size: 50_000)
        try writeFile(at: caches.appendingPathComponent("com.apple.Safari/data.bin"), size: 50_000)

        let cleaner = CacheCleaner(denylist: permissiveDenylist)
        let items = cleaner.scan(home: home)
        XCTAssertTrue(items.contains { $0.displayName == "com.acme.demo" })
        XCTAssertFalse(items.contains { $0.displayName.hasPrefix("com.apple.") })
    }

    func testDevDependencyRequiresManifestAdjacency() throws {
        let project = tempRoot.appendingPathComponent("MyApp")
        let random = tempRoot.appendingPathComponent("Random")
        let realModules = project.appendingPathComponent("node_modules")
        let orphanModules = random.appendingPathComponent("node_modules")

        try FileManager.default.createDirectory(at: realModules, withIntermediateDirectories: true)
        try FileManager.default.createDirectory(at: orphanModules, withIntermediateDirectories: true)
        try "{}".write(to: project.appendingPathComponent("package.json"), atomically: true, encoding: .utf8)
        try writeFile(at: realModules.appendingPathComponent("pkg/index.js"), size: 10_000)
        try writeFile(at: orphanModules.appendingPathComponent("pkg/index.js"), size: 10_000)

        let scanner = DevDependencyScanner(denylist: permissiveDenylist)
        XCTAssertTrue(scanner.isManifestAdjacentDependency(parent: project, folderName: "node_modules"))
        XCTAssertFalse(scanner.isManifestAdjacentDependency(parent: random, folderName: "node_modules"))

        let found = scanner.scanProjectFolders(roots: [tempRoot], maxDepth: 3)
        XCTAssertTrue(
            found.contains { standardized($0.path) == standardized(realModules.path) },
            "Expected to find \(realModules.path) in \(found.map(\.path))"
        )
        XCTAssertFalse(found.contains { standardized($0.path) == standardized(orphanModules.path) })
    }

    func testStorageOverviewFormats() {
        let overview = StorageOverview(
            totalBytes: 1_000_000_000,
            availableBytes: 250_000_000,
            categories: [
                StorageCategorySize(kind: .caches, byteSize: 10_000),
                StorageCategorySize(kind: .trash, byteSize: 5_000)
            ]
        )
        XCTAssertEqual(overview.usedBytes, 750_000_000)
        XCTAssertEqual(overview.usedFraction, 0.75, accuracy: 0.001)
        XCTAssertFalse(overview.formattedAvailable.isEmpty)
    }

    func testVideoPathHelper() {
        XCTAssertTrue(LargeFileScanner.isVideoPath("/tmp/a.MOV"))
        XCTAssertTrue(LargeFileScanner.isVideoPath("/tmp/a.mp4"))
        XCTAssertFalse(LargeFileScanner.isVideoPath("/tmp/a.pdf"))
    }
}
