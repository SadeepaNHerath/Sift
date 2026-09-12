import XCTest
import CoreGraphics
import ImageIO
import UniformTypeIdentifiers
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
        let results = scanner.scan(roots: [downloads], minimumBytes: 100_000, maxFiles: 100, preferSpotlight: false)
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
        XCTAssertTrue(LargeFileScanner.isImagePath("/tmp/a.HEIC"))
        XCTAssertTrue(LargeFileScanner.isImagePath("/tmp/a.png"))
        XCTAssertFalse(LargeFileScanner.isImagePath("/tmp/a.mov"))
    }

    func testLargeFileScannerFileManagerFallback() throws {
        let downloads = tempRoot.appendingPathComponent("Downloads")
        try FileManager.default.createDirectory(at: downloads, withIntermediateDirectories: true)
        let big = downloads.appendingPathComponent("photo.png")
        try writeFile(at: big, size: 150_000)

        let scanner = LargeFileScanner(denylist: permissiveDenylist)
        let results = scanner.scan(
            roots: [downloads],
            minimumBytes: 100_000,
            maxFiles: 100,
            preferSpotlight: false
        )
        XCTAssertEqual(results.count, 1)
        XCTAssertTrue(results[0].isImage)
        XCTAssertFalse(results[0].isVideo)
    }

    func testImageCompressorRoundTrip() throws {
        let sourceURL = try writeLargeJPEG(named: "sample.jpg", width: 800, height: 600)
        let originalSize = try XCTUnwrap(
            (try? sourceURL.resourceValues(forKeys: [.fileSizeKey]).fileSize).map(Int64.init)
        )
        XCTAssertGreaterThan(originalSize, 50_000)

        let compressor = ImageCompressor(fileManager: .default, jpegQuality: 0.4, heicQuality: 0.4)
        let result = compressor.compress(path: sourceURL.path)

        XCTAssertTrue(result.success, result.errorMessage ?? "unknown")
        XCTAssertTrue(FileManager.default.fileExists(atPath: result.outputPath))
        XCTAssertLessThan(result.compressedBytes, result.originalBytes)
        XCTAssertFalse(FileManager.default.fileExists(atPath: sourceURL.path), "Original should be in Trash")
    }

    func testCLICleanupMissingBinaryDoesNotCrash() {
        let runner = CLICleanupRunner(pathResolver: { _ in nil })
        for tool in CLICleanupRunner.ToolID.allCases {
            let result = runner.run(tool)
            XCTAssertFalse(result.success)
            XCTAssertTrue(
                result.summary.lowercased().contains("not installed") || result.summary.lowercased().contains("path"),
                result.summary
            )
        }
        let actions = runner.availableActions()
        XCTAssertEqual(actions.count, 3)
        XCTAssertTrue(actions.allSatisfy { !$0.isAvailable })
    }

    func testVideoCompressionResultSavingsPercent() {
        let result = VideoCompressionResult(
            originalPath: "/a",
            outputPath: "/b",
            originalBytes: 1000,
            compressedBytes: 250,
            success: true
        )
        XCTAssertEqual(result.savedBytes, 750)
        XCTAssertEqual(result.savedPercent, 75)
    }

    func testLargeFileMergeDedupeAndFileManagerWins() {
        let shared = "/tmp/shared-big.mov"
        let spotlightOnly = LargeFileItem(path: shared, byteSize: 100, isVideo: true, isImage: false)
        let enumeratedShared = LargeFileItem(path: shared, byteSize: 200, isVideo: true, isImage: false)
        let fmOnly = LargeFileItem(path: "/tmp/other.zip", byteSize: 150, isVideo: false, isImage: false)

        let merged = LargeFileScanner.merge(
            spotlight: [spotlightOnly],
            enumerated: [enumeratedShared, fmOnly],
            maxFiles: 10
        )
        XCTAssertEqual(merged.count, 2)
        XCTAssertEqual(merged.first?.byteSize, 200, "FileManager size should win on collision")
        XCTAssertTrue(merged.contains { $0.path.hasSuffix("other.zip") })
    }

    func testDefaultRootsIncludePictures() {
        let roots = LargeFileScanner(denylist: permissiveDenylist).defaultRoots(home: tempRoot)
        XCTAssertTrue(roots.contains { $0.lastPathComponent == "Pictures" })
        XCTAssertTrue(roots.contains { $0.lastPathComponent == "Downloads" })
        let expanded = LargeFileScanner(denylist: permissiveDenylist)
            .defaultRoots(home: tempRoot, includeExpandedHome: true)
        XCTAssertTrue(expanded.contains { $0.lastPathComponent == "Music" })
        XCTAssertTrue(expanded.contains { standardized($0.path) == standardized(tempRoot.path) })
    }

    func testImageCompressorCancelDoesNotTrash() throws {
        let sourceURL = try writeLargeJPEG(named: "cancel-me.jpg", width: 1600, height: 1200)
        let compressor = ImageCompressor(
            fileManager: .default,
            denylist: permissiveDenylist,
            jpegQuality: 0.85,
            heicQuality: 0.85
        )

        let expect = expectation(description: "compress finished")
        var result: VideoCompressionResult?
        DispatchQueue.global(qos: .userInitiated).async {
            // Automatic encodes HEIC then JPEG — gives cancel a window before trash.
            result = compressor.compress(path: sourceURL.path, format: .automatic)
            expect.fulfill()
        }
        DispatchQueue.global().async {
            for _ in 0..<200 {
                Thread.sleep(forTimeInterval: 0.005)
                compressor.cancelActive()
            }
        }
        wait(for: [expect], timeout: 60)
        let final = try XCTUnwrap(result)
        XCTAssertTrue(final.wasCancelled, "Expected cancel, got: \(final.errorMessage ?? "success")")
        XCTAssertTrue(FileManager.default.fileExists(atPath: sourceURL.path), "Original must remain after cancel")
        XCTAssertFalse(final.success)
    }

    func testImageCompressorRefusesDeniedPath() {
        let compressor = ImageCompressor(denylist: SafetyDenylist.shared)
        let result = compressor.compress(path: "/System/Library/CoreServices/SystemVersion.plist", format: .jpeg)
        XCTAssertFalse(result.success)
        XCTAssertFalse(result.wasCancelled)
        XCTAssertNotNil(result.errorMessage)
    }

    func testVideoCompressorCancelWithNoSessionDoesNotCrash() {
        VideoCompressor.shared.cancelActiveExport()
    }

    func testCLICleanupFindsBinaryOutsideDefaultPATH() throws {
        let binDir = tempRoot.appendingPathComponent("custom-bin")
        try FileManager.default.createDirectory(at: binDir, withIntermediateDirectories: true)
        let brew = binDir.appendingPathComponent("brew")
        try "#!/bin/sh\necho ok\n".write(to: brew, atomically: true, encoding: .utf8)
        try FileManager.default.setAttributes([.posixPermissions: 0o755], ofItemAtPath: brew.path)

        let runner = CLICleanupRunner(pathResolver: { name in
            let candidate = binDir.appendingPathComponent(name).path
            return FileManager.default.isExecutableFile(atPath: candidate) ? candidate : nil
        })
        XCTAssertTrue(runner.availableActions().contains { $0.toolID == "brew" && $0.isAvailable })
        let result = runner.run(.brew)
        XCTAssertTrue(result.success, result.summary)
    }

    func testCLICleanupTimeout() {
        let runner = CLICleanupRunner(timeout: 1)
        let result = runner.runProcess(toolID: "sleep", launchPath: "/bin/sleep", arguments: ["5"])
        XCTAssertFalse(result.success)
        XCTAssertTrue(result.summary.lowercased().contains("timed out"), result.summary)
    }

    func testApproximateCategoryFormatting() {
        let exact = StorageCategorySize(kind: .caches, byteSize: 1024, isApproximate: false)
        let approx = StorageCategorySize(kind: .caches, byteSize: 1024, isApproximate: true)
        XCTAssertFalse(exact.formattedSize.hasPrefix("~"))
        XCTAssertTrue(approx.formattedSize.hasPrefix("~"))
    }

    private func writeLargeJPEG(named name: String, width: Int, height: Int) throws -> URL {
        let url = tempRoot.appendingPathComponent(name)
        let colorSpace = CGColorSpaceCreateDeviceRGB()
        var pixels = [UInt8](repeating: 0, count: width * height * 4)
        var state: UInt64 = 0xC0FFEE
        for i in stride(from: 0, to: pixels.count, by: 4) {
            state = state &* 6364136223846793005 &+ 1
            let v = UInt8(truncatingIfNeeded: state >> 56)
            pixels[i] = v
            pixels[i + 1] = UInt8(truncatingIfNeeded: state >> 48)
            pixels[i + 2] = UInt8(truncatingIfNeeded: state >> 40)
            pixels[i + 3] = 255
        }
        guard let context = CGContext(
            data: &pixels,
            width: width,
            height: height,
            bitsPerComponent: 8,
            bytesPerRow: width * 4,
            space: colorSpace,
            bitmapInfo: CGImageAlphaInfo.noneSkipLast.rawValue
        ), let image = context.makeImage() else {
            throw NSError(domain: "StorageToolkitTests", code: 1)
        }
        guard let destination = CGImageDestinationCreateWithURL(
            url as CFURL,
            UTType.jpeg.identifier as CFString,
            1,
            nil
        ) else {
            throw NSError(domain: "StorageToolkitTests", code: 2)
        }
        let options: [CFString: Any] = [kCGImageDestinationLossyCompressionQuality: 1.0]
        CGImageDestinationAddImage(destination, image, options as CFDictionary)
        XCTAssertTrue(CGImageDestinationFinalize(destination))
        return url
    }
}
