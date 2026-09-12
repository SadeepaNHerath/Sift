import Foundation
import ImageIO
import UniformTypeIdentifiers

/// Compresses large images with ImageIO (HEIC preferred, JPEG fallback).
public final class ImageCompressor: @unchecked Sendable {
    public static let shared = ImageCompressor()

    private let fileManager: FileManager
    private let denylist: SafetyDenylist
    private let jpegQuality: CGFloat
    private let heicQuality: CGFloat
    private let lock = NSLock()
    private var cancelRequested = false
    private var activeGeneration: UInt64 = 0

    public init(
        fileManager: FileManager = .default,
        denylist: SafetyDenylist = .shared,
        jpegQuality: CGFloat = 0.7,
        heicQuality: CGFloat = 0.7
    ) {
        self.fileManager = fileManager
        self.denylist = denylist
        self.jpegQuality = jpegQuality
        self.heicQuality = heicQuality
    }

    /// Cancel an in-flight compress. Trash will not run after this returns.
    public func cancelActive() {
        lock.lock()
        cancelRequested = true
        lock.unlock()
    }

    /// Estimate compressed size without trashing the original.
    public func estimate(path: String, format: ImageOutputFormat = .automatic) -> Int64? {
        if let reason = denylist.denyReason(for: path) {
            _ = reason
            return nil
        }
        guard fileManager.fileExists(atPath: path), LargeFileScanner.isImagePath(path) else { return nil }
        guard let source = CGImageSourceCreateWithURL(URL(fileURLWithPath: path) as CFURL, nil) else { return nil }

        let originalSize = fileSize(URL(fileURLWithPath: path))
        let tempDir = fileManager.temporaryDirectory.appendingPathComponent("SafeUninstallerImageEstimate", isDirectory: true)
        try? fileManager.createDirectory(at: tempDir, withIntermediateDirectories: true)

        var candidates: [Int64] = []
        if format == .automatic || format == .heic {
            let heicTemp = tempDir.appendingPathComponent(UUID().uuidString + ".heic")
            if writeDestination(source: source, to: heicTemp, type: UTType.heic.identifier as CFString, quality: heicQuality) {
                let size = fileSize(heicTemp)
                if size > 0 { candidates.append(size) }
            }
            try? fileManager.removeItem(at: heicTemp)
        }
        if format == .automatic || format == .jpeg {
            let jpegTemp = tempDir.appendingPathComponent(UUID().uuidString + ".jpg")
            if writeDestination(source: source, to: jpegTemp, type: UTType.jpeg.identifier as CFString, quality: jpegQuality) {
                let size = fileSize(jpegTemp)
                if size > 0 { candidates.append(size) }
            }
            try? fileManager.removeItem(at: jpegTemp)
        }

        guard let best = candidates.min(), best < originalSize else { return nil }
        return best
    }

    /// Compress one image. On success: original → Trash, compressed file written beside it.
    public func compress(path: String, format: ImageOutputFormat = .automatic) -> VideoCompressionResult {
        lock.lock()
        cancelRequested = false
        activeGeneration &+= 1
        let generation = activeGeneration
        lock.unlock()

        let url = URL(fileURLWithPath: path)
        let originalSize = (try? url.resourceValues(forKeys: [.fileSizeKey]).fileSize).map(Int64.init) ?? 0

        if let reason = denylist.denyReason(for: path) {
            return failure(path: path, original: originalSize, message: reason)
        }

        guard fileManager.fileExists(atPath: path) else {
            return failure(path: path, original: originalSize, message: "File not found")
        }
        guard LargeFileScanner.isImagePath(path) else {
            return failure(path: path, original: originalSize, message: "Not a supported image")
        }

        guard let source = CGImageSourceCreateWithURL(url as CFURL, nil) else {
            return failure(path: path, original: originalSize, message: "Could not read image")
        }

        if isCancelled(generation: generation) {
            return cancelled(path: path, original: originalSize)
        }

        let tempDir = fileManager.temporaryDirectory.appendingPathComponent("SafeUninstallerImage", isDirectory: true)
        try? fileManager.createDirectory(at: tempDir, withIntermediateDirectories: true)

        let heicTemp = tempDir.appendingPathComponent(UUID().uuidString + ".heic")
        let jpegTemp = tempDir.appendingPathComponent(UUID().uuidString + ".jpg")
        try? fileManager.removeItem(at: heicTemp)
        try? fileManager.removeItem(at: jpegTemp)

        var heicOK = false
        var jpegOK = false

        if format == .automatic || format == .heic {
            heicOK = writeDestination(
                source: source,
                to: heicTemp,
                type: UTType.heic.identifier as CFString,
                quality: heicQuality
            )
            if isCancelled(generation: generation) {
                cleanupTemps(heicTemp, jpegTemp)
                return cancelled(path: path, original: originalSize)
            }
        }

        if format == .automatic || format == .jpeg {
            jpegOK = writeDestination(
                source: source,
                to: jpegTemp,
                type: UTType.jpeg.identifier as CFString,
                quality: jpegQuality
            )
            if isCancelled(generation: generation) {
                cleanupTemps(heicTemp, jpegTemp)
                return cancelled(path: path, original: originalSize)
            }
        }

        let heicSize = heicOK ? fileSize(heicTemp) : Int64.max
        let jpegSize = jpegOK ? fileSize(jpegTemp) : Int64.max

        let chosenURL: URL
        let chosenSize: Int64
        let outExt: String

        switch format {
        case .heic:
            guard heicSize < originalSize && heicSize > 0 && heicSize != Int64.max else {
                cleanupTemps(heicTemp, jpegTemp)
                return failure(
                    path: path,
                    original: originalSize,
                    message: "Compressed file was not smaller — original kept",
                    compressed: heicSize == Int64.max ? 0 : heicSize
                )
            }
            chosenURL = heicTemp
            chosenSize = heicSize
            outExt = "heic"
            try? fileManager.removeItem(at: jpegTemp)
        case .jpeg:
            guard jpegSize < originalSize && jpegSize > 0 && jpegSize != Int64.max else {
                cleanupTemps(heicTemp, jpegTemp)
                return failure(
                    path: path,
                    original: originalSize,
                    message: "Compressed file was not smaller — original kept",
                    compressed: jpegSize == Int64.max ? 0 : jpegSize
                )
            }
            chosenURL = jpegTemp
            chosenSize = jpegSize
            outExt = "jpg"
            try? fileManager.removeItem(at: heicTemp)
        case .automatic:
            if heicSize < jpegSize && heicSize < originalSize && heicSize > 0 {
                chosenURL = heicTemp
                chosenSize = heicSize
                outExt = "heic"
                try? fileManager.removeItem(at: jpegTemp)
            } else if jpegSize < originalSize && jpegSize > 0 {
                chosenURL = jpegTemp
                chosenSize = jpegSize
                outExt = "jpg"
                try? fileManager.removeItem(at: heicTemp)
            } else {
                cleanupTemps(heicTemp, jpegTemp)
                return failure(
                    path: path,
                    original: originalSize,
                    message: "Compressed file was not smaller — original kept",
                    compressed: min(
                        heicSize == Int64.max ? originalSize : heicSize,
                        jpegSize == Int64.max ? originalSize : jpegSize
                    )
                )
            }
        }

        // Final cancel gate — never trash after cancel.
        if isCancelled(generation: generation) {
            try? fileManager.removeItem(at: chosenURL)
            return cancelled(path: path, original: originalSize)
        }

        let parent = url.deletingLastPathComponent()
        let stem = url.deletingPathExtension().lastPathComponent
        var finalURL = parent.appendingPathComponent("\(stem).\(outExt)")

        do {
            var resulting: NSURL?
            try fileManager.trashItem(at: url, resultingItemURL: &resulting)

            if fileManager.fileExists(atPath: finalURL.path) {
                finalURL = parent.appendingPathComponent("\(stem)_compressed.\(outExt)")
            }
            try fileManager.moveItem(at: chosenURL, to: finalURL)
            return VideoCompressionResult(
                originalPath: path,
                outputPath: finalURL.path,
                originalBytes: originalSize,
                compressedBytes: chosenSize,
                success: true
            )
        } catch {
            try? fileManager.removeItem(at: chosenURL)
            return failure(
                path: path,
                original: originalSize,
                message: error.localizedDescription,
                compressed: chosenSize
            )
        }
    }

    private func isCancelled(generation: UInt64) -> Bool {
        lock.lock()
        defer { lock.unlock() }
        return cancelRequested || generation != activeGeneration
    }

    private func cleanupTemps(_ urls: URL...) {
        for url in urls {
            try? fileManager.removeItem(at: url)
        }
    }

    private func cancelled(path: String, original: Int64) -> VideoCompressionResult {
        VideoCompressionResult(
            originalPath: path,
            outputPath: "",
            originalBytes: original,
            compressedBytes: 0,
            success: false,
            errorMessage: "Cancelled",
            wasCancelled: true
        )
    }

    private func writeDestination(
        source: CGImageSource,
        to url: URL,
        type: CFString,
        quality: CGFloat
    ) -> Bool {
        guard let destination = CGImageDestinationCreateWithURL(url as CFURL, type, 1, nil) else {
            return false
        }
        let options: [CFString: Any] = [
            kCGImageDestinationLossyCompressionQuality: quality
        ]
        CGImageDestinationAddImageFromSource(destination, source, 0, options as CFDictionary)
        return CGImageDestinationFinalize(destination)
    }

    private func fileSize(_ url: URL) -> Int64 {
        (try? url.resourceValues(forKeys: [.fileSizeKey]).fileSize).map(Int64.init) ?? 0
    }

    private func failure(
        path: String,
        original: Int64,
        message: String,
        compressed: Int64 = 0
    ) -> VideoCompressionResult {
        VideoCompressionResult(
            originalPath: path,
            outputPath: "",
            originalBytes: original,
            compressedBytes: compressed,
            success: false,
            errorMessage: message
        )
    }
}
