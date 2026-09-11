import Foundation
import AVFoundation

/// Compresses videos with AVFoundation (HEVC preferred, H.264 fallback).
public final class VideoCompressor: @unchecked Sendable {
    public static let shared = VideoCompressor()

    private let fileManager: FileManager

    public init(fileManager: FileManager = .default) {
        self.fileManager = fileManager
    }

    public enum ProgressEvent: Sendable {
        case started(path: String)
        case fraction(path: String, value: Double)
        case finished(VideoCompressionResult)
    }

    /// Compress one video. On success: original → Trash, compressed file takes its place.
    public func compress(
        path: String,
        progress: ((Double) -> Void)? = nil
    ) async -> VideoCompressionResult {
        let url = URL(fileURLWithPath: path)
        let originalSize = (try? url.resourceValues(forKeys: [.fileSizeKey]).fileSize).map(Int64.init) ?? 0

        guard fileManager.fileExists(atPath: path) else {
            return VideoCompressionResult(
                originalPath: path,
                outputPath: "",
                originalBytes: originalSize,
                compressedBytes: 0,
                success: false,
                errorMessage: "File not found"
            )
        }

        let asset = AVURLAsset(url: url)
        let preset = await preferredPreset(for: asset)
        guard let session = AVAssetExportSession(asset: asset, presetName: preset) else {
            return VideoCompressionResult(
                originalPath: path,
                outputPath: "",
                originalBytes: originalSize,
                compressedBytes: 0,
                success: false,
                errorMessage: "Could not create export session"
            )
        }

        let tempDir = fileManager.temporaryDirectory.appendingPathComponent("SafeUninstallerVideo", isDirectory: true)
        try? fileManager.createDirectory(at: tempDir, withIntermediateDirectories: true)
        let outExt = preset.contains("HEVC") || path.lowercased().hasSuffix(".mp4") ? "mp4" : "mov"
        let tempOut = tempDir.appendingPathComponent(UUID().uuidString + "." + outExt)
        try? fileManager.removeItem(at: tempOut)

        session.outputURL = tempOut
        session.outputFileType = outExt == "mp4" ? .mp4 : .mov
        session.shouldOptimizeForNetworkUse = true

        let progressTask = Task {
            while !Task.isCancelled {
                let value = Double(session.progress)
                progress?(value)
                if session.status != .exporting && session.status != .waiting { break }
                try? await Task.sleep(nanoseconds: 200_000_000)
            }
        }

        await withCheckedContinuation { (continuation: CheckedContinuation<Void, Never>) in
            session.exportAsynchronously {
                continuation.resume()
            }
        }
        progressTask.cancel()
        progress?(1.0)

        guard session.status == .completed, fileManager.fileExists(atPath: tempOut.path) else {
            let message = session.error?.localizedDescription ?? "Export failed"
            try? fileManager.removeItem(at: tempOut)
            return VideoCompressionResult(
                originalPath: path,
                outputPath: "",
                originalBytes: originalSize,
                compressedBytes: 0,
                success: false,
                errorMessage: message
            )
        }

        let compressedSize = (try? tempOut.resourceValues(forKeys: [.fileSizeKey]).fileSize).map(Int64.init) ?? 0
        if compressedSize <= 0 || compressedSize >= originalSize {
            try? fileManager.removeItem(at: tempOut)
            return VideoCompressionResult(
                originalPath: path,
                outputPath: "",
                originalBytes: originalSize,
                compressedBytes: compressedSize,
                success: false,
                errorMessage: "Compressed file was not smaller — original kept"
            )
        }

        // Destination: same directory, stem + _compressed + ext (then rename over after trash)
        let parent = url.deletingLastPathComponent()
        let stem = url.deletingPathExtension().lastPathComponent
        let finalURL = parent.appendingPathComponent("\(stem).\(outExt)")

        do {
            var resulting: NSURL?
            try fileManager.trashItem(at: url, resultingItemURL: &resulting)

            if fileManager.fileExists(atPath: finalURL.path) && finalURL.path != path {
                // Unlikely collision with different extension — use unique name
                let alt = parent.appendingPathComponent("\(stem)_compressed.\(outExt)")
                try fileManager.moveItem(at: tempOut, to: alt)
                return VideoCompressionResult(
                    originalPath: path,
                    outputPath: alt.path,
                    originalBytes: originalSize,
                    compressedBytes: compressedSize,
                    success: true
                )
            }

            try fileManager.moveItem(at: tempOut, to: finalURL)
            return VideoCompressionResult(
                originalPath: path,
                outputPath: finalURL.path,
                originalBytes: originalSize,
                compressedBytes: compressedSize,
                success: true
            )
        } catch {
            try? fileManager.removeItem(at: tempOut)
            return VideoCompressionResult(
                originalPath: path,
                outputPath: "",
                originalBytes: originalSize,
                compressedBytes: compressedSize,
                success: false,
                errorMessage: error.localizedDescription
            )
        }
    }

    private func preferredPreset(for asset: AVAsset) async -> String {
        let hevc = AVAssetExportPresetHEVCHighestQuality
        let h264 = AVAssetExportPreset1920x1080
        let compatible = AVAssetExportSession.exportPresets(compatibleWith: asset)
        if compatible.contains(hevc) { return hevc }
        if compatible.contains(h264) { return h264 }
        if compatible.contains(AVAssetExportPresetMediumQuality) { return AVAssetExportPresetMediumQuality }
        return AVAssetExportPresetPassthrough
    }
}
