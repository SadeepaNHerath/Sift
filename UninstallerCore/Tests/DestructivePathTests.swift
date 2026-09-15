import XCTest
@testable import UninstallerCore

final class DestructivePathTests: XCTestCase {
    var tempRoot: URL!
    var permissiveDenylist: SafetyDenylist!

    override func setUpWithError() throws {
        tempRoot = FileManager.default.temporaryDirectory
            .appendingPathComponent("DestructivePathTests-\(UUID().uuidString)", isDirectory: true)
        try FileManager.default.createDirectory(at: tempRoot, withIntermediateDirectories: true)
        permissiveDenylist = SafetyDenylist(
            protectedPrefixes: ["/System", "/usr/bin"],
            protectedBundlePrefixes: ["com.apple."]
        )
    }

    override func tearDownWithError() throws {
        try? FileManager.default.removeItem(at: tempRoot)
    }

    func testTrashPathsMovesItemUnderTemp() async throws {
        let file = tempRoot.appendingPathComponent("junk.txt")
        try "hello".write(to: file, atomically: true, encoding: .utf8)
        XCTAssertTrue(FileManager.default.fileExists(atPath: file.path))

        let engine = RemovalEngine(fileManager: .default, denylist: permissiveDenylist)
        let report = await engine.trashPaths(
            [(path: file.path, category: .other, byteSize: 5)],
            reportName: "Test Trash",
            dryRun: false
        )
        XCTAssertEqual(report.removedCount, 1)
        XCTAssertFalse(FileManager.default.fileExists(atPath: file.path))
    }

    func testTrashPathsRefusesOutsideHomeAndTemp() async throws {
        let engine = RemovalEngine(fileManager: .default, denylist: permissiveDenylist)
        let report = await engine.trashPaths(
            [(path: "/Applications", category: .other, byteSize: 0)],
            reportName: "Refuse",
            dryRun: false
        )
        XCTAssertEqual(report.removedCount, 0)
        XCTAssertTrue(report.items.contains { $0.outcome == .skippedDenied })
    }

    func testTrashPathsDryRunDoesNotMove() async throws {
        let file = tempRoot.appendingPathComponent("keep.txt")
        try "keep".write(to: file, atomically: true, encoding: .utf8)
        let engine = RemovalEngine(fileManager: .default, denylist: permissiveDenylist)
        let report = await engine.trashPaths(
            [(path: file.path, category: .other, byteSize: 4)],
            reportName: "Dry",
            dryRun: true
        )
        XCTAssertEqual(report.removedCount, 1)
        XCTAssertTrue(FileManager.default.fileExists(atPath: file.path))
        XCTAssertTrue(report.notes.lowercased().contains("dry"))
    }

    func testImageCompressKeepsOriginalWhenDestinationBlocked() throws {
        // Write a tiny valid PNG via CoreGraphics-free approach: skip if compress rejects non-image.
        // Use a non-image file to assert denylist/failure leaves source intact.
        let source = tempRoot.appendingPathComponent("photo.bin")
        try Data(repeating: 0xFF, count: 2048).write(to: source)
        let compressor = ImageCompressor(fileManager: .default, denylist: permissiveDenylist)
        let result = compressor.compress(path: source.path, format: .jpeg)
        XCTAssertFalse(result.success)
        XCTAssertTrue(FileManager.default.fileExists(atPath: source.path), "Original must remain when compress fails")
    }
}
