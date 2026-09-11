import Foundation
import ImageIO
import UniformTypeIdentifiers

/// Compresses large images with ImageIO (HEIC preferred, JPEG fallback).
public final class ImageCompressor: @unchecked Sendable {
    public static let shared = ImageCompressor()

    private let fileManager: FileManager
    private let jpegQuality: CGFloat
    private let heicQuality: CGFloat

    public init(
        fileManager: FileManager = .default,
        jpegQuality: CGFloat = 0.7,
        heicQuality: CGFloat = 0.7
    ) {
        self.fileManager = fileManager
        self.jpegQuality = jpegQuality
        self.heicQuality = heicQuality
    }

    /// Compress one image. On success: original → Trash, compressed file written beside it.
    public func compress(path: String) -> VideoCompressionResult {
        let url = URL(fileURLWithPath: path)
        let originalSize = (try? url.resourceValues(forKeys: [.fileSizeKey]).fileSize).map(Int64.init) ?? 0

        guard fileManager.fileExists(atPath: path) else {
            return failure(path: path, original: originalSize, message: "File not found")
        }
        guard LargeFileScanner.isImagePath(path) else {
            return failure(path: path, original: originalSize, message: "Not a supported image")
        }

        guard let source = CGImageSourceCreateWithURL(url as CFURL, nil) else {
            return failure(path: path, original: originalSize, message: "Could not read image")
        }

        let tempDir = fileManager.temporaryDirectory.appendingPathComponent("SafeUninstallerImage", isDirectory: true)
        try? fileManager.createDirectory(at: tempDir, withIntermediateDirectories: true)

        let heicTemp = tempDir.appendingPathComponent(UUID().uuidString + ".heic")
        let jpegTemp = tempDir.appendingPathComponent(UUID().uuidString + ".jpg")
        try? fileManager.removeItem(at: heicTemp)
        try? fileManager.removeItem(at: jpegTemp)

        let heicOK = writeDestination(
            source: source,
            to: heicTemp,
            type: UTType.heic.identifier as CFString,
            quality: heicQuality
        )
        let jpegOK = writeDestination(
            source: source,
            to: jpegTemp,
            type: UTType.jpeg.identifier as CFString,
            quality: jpegQuality
        )

        let heicSize = heicOK ? fileSize(heicTemp) : Int64.max
        let jpegSize = jpegOK ? fileSize(jpegTemp) : Int64.max

        let chosenURL: URL
        let chosenSize: Int64
        let outExt: String

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
            try? fileManager.removeItem(at: heicTemp)
            try? fileManager.removeItem(at: jpegTemp)
            return failure(
                path: path,
                original: originalSize,
                message: "Compressed file was not smaller — original kept",
                compressed: min(heicSize == Int64.max ? originalSize : heicSize, jpegSize == Int64.max ? originalSize : jpegSize)
            )
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
