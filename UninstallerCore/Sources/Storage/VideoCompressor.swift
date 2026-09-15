import Foundation
import AVFoundation

/// Compresses videos with AVFoundation (HEVC preferred, H.264 fallback).
public final class VideoCompressor: @unchecked Sendable {
    public static let shared = VideoCompressor()

    private let fileManager: FileManager
    private let denylist: SafetyDenylist
    private let lock = NSLock()
    private var activeSession: AVAssetExportSession?

    public init(fileManager: FileManager = .default, denylist: SafetyDenylist = .shared) {
        self.fileManager = fileManager
        self.denylist = denylist
    }

    public enum ProgressEvent: Sendable {
        case started(path: String)
        case fraction(path: String, value: Double)
        case finished(VideoCompressionResult)
    }

    /// Cancel an in-flight export (e.g. user navigates away).
    public func cancelActiveExport() {
        lock.lock()
        let session = activeSession
        lock.unlock()
        session?.cancelExport()
    }

    /// Sample dry-run estimate: export first ≤12s with the same preset, then extrapolate.
    /// Falls back to bitrate × scale heuristic on failure / timeout (~45s).
    public func estimate(path: String, quality: VideoQualityPreset = .balanced) async -> Int64? {
        if denylist.denyReason(for: path) != nil { return nil }
        let url = URL(fileURLWithPath: path)
        guard fileManager.fileExists(atPath: path) else { return nil }
        let originalSize = (try? url.resourceValues(forKeys: [.fileSizeKey]).fileSize).map(Int64.init) ?? 0
        guard originalSize > 0 else { return nil }

        if let sample = await sampleExportEstimate(path: path, url: url, originalSize: originalSize, quality: quality) {
            return sample
        }
        return await heuristicEstimate(url: url, originalSize: originalSize, quality: quality)
    }

    private func sampleExportEstimate(
        path: String,
        url: URL,
        originalSize: Int64,
        quality: VideoQualityPreset
    ) async -> Int64? {
        let asset = AVURLAsset(url: url)
        let duration: CMTime
        do {
            duration = try await asset.load(.duration)
        } catch {
            return nil
        }
        let fullSeconds = CMTimeGetSeconds(duration)
        guard fullSeconds.isFinite, fullSeconds > 0 else { return nil }

        let sampleSeconds = min(12.0, fullSeconds)
        let sampleDuration = CMTime(seconds: sampleSeconds, preferredTimescale: 600)
        let timeRange = CMTimeRange(start: .zero, duration: sampleDuration)

        let preset = await preferredPreset(for: asset, quality: quality)
        guard let session = AVAssetExportSession(asset: asset, presetName: preset) else { return nil }

        lock.lock()
        activeSession = session
        lock.unlock()
        defer {
            lock.lock()
            if activeSession === session { activeSession = nil }
            lock.unlock()
        }

        let tempDir = fileManager.temporaryDirectory.appendingPathComponent("SafeUninstallerVideoEstimate", isDirectory: true)
        try? fileManager.createDirectory(at: tempDir, withIntermediateDirectories: true)
        let outExt = preset.contains("HEVC") || path.lowercased().hasSuffix(".mp4") ? "mp4" : "mov"
        let tempOut = tempDir.appendingPathComponent(UUID().uuidString + "." + outExt)
        try? fileManager.removeItem(at: tempOut)

        session.outputURL = tempOut
        session.outputFileType = outExt == "mp4" ? .mp4 : .mov
        session.shouldOptimizeForNetworkUse = true
        session.timeRange = timeRange

        let finished = await withTaskGroup(of: Bool.self) { group -> Bool in
            group.addTask {
                await withCheckedContinuation { (continuation: CheckedContinuation<Void, Never>) in
                    session.exportAsynchronously {
                        continuation.resume()
                    }
                }
                return true
            }
            group.addTask {
                try? await Task.sleep(nanoseconds: 45_000_000_000)
                session.cancelExport()
                return false
            }
            let first = await group.next() ?? false
            group.cancelAll()
            return first && session.status == .completed
        }

        defer { try? fileManager.removeItem(at: tempOut) }

        if session.status == .cancelled { return nil }
        guard finished, session.status == .completed, fileManager.fileExists(atPath: tempOut.path) else {
            return nil
        }
        let sampleBytes = (try? tempOut.resourceValues(forKeys: [.fileSizeKey]).fileSize).map(Int64.init) ?? 0
        guard sampleBytes > 0 else { return nil }

        let extrapolated = Int64(Double(sampleBytes) * (fullSeconds / sampleSeconds))
        return min(originalSize - 1, max(1, extrapolated))
    }

    private func heuristicEstimate(url: URL, originalSize: Int64, quality: VideoQualityPreset) async -> Int64? {
        let asset = AVURLAsset(url: url)
        do {
            let duration = try await asset.load(.duration)
            let seconds = CMTimeGetSeconds(duration)
            guard seconds.isFinite, seconds > 0 else {
                return max(1, Int64(Double(originalSize) * quality.estimateScale))
            }
            let tracks = try await asset.loadTracks(withMediaType: .video)
            var bitsPerSecond: Float = 0
            for track in tracks {
                let rate = try await track.load(.estimatedDataRate)
                if rate > 0 { bitsPerSecond += rate }
            }
            if bitsPerSecond <= 0 {
                return max(1, Int64(Double(originalSize) * quality.estimateScale))
            }
            let estimated = Int64(Double(bitsPerSecond) * seconds / 8.0 * quality.estimateScale)
            return min(originalSize - 1, max(1, estimated))
        } catch {
            return max(1, Int64(Double(originalSize) * quality.estimateScale))
        }
    }

    /// Compress one video. On success: original → Trash, compressed file takes its place.
    public func compress(
        path: String,
        quality: VideoQualityPreset = .balanced,
        progress: ((Double) -> Void)? = nil
    ) async -> VideoCompressionResult {
        let url = URL(fileURLWithPath: path)
        let originalSize = (try? url.resourceValues(forKeys: [.fileSizeKey]).fileSize).map(Int64.init) ?? 0

        if let reason = denylist.denyReason(for: path) {
            return VideoCompressionResult(
                originalPath: path,
                outputPath: "",
                originalBytes: originalSize,
                compressedBytes: 0,
                success: false,
                errorMessage: reason
            )
        }

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
        let preset = await preferredPreset(for: asset, quality: quality)
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

        lock.lock()
        activeSession = session
        lock.unlock()
        defer {
            lock.lock()
            if activeSession === session { activeSession = nil }
            lock.unlock()
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
        progress?(session.status == .cancelled ? Double(session.progress) : 1.0)

        if session.status == .cancelled || Task.isCancelled {
            try? fileManager.removeItem(at: tempOut)
            return VideoCompressionResult(
                originalPath: path,
                outputPath: "",
                originalBytes: originalSize,
                compressedBytes: 0,
                success: false,
                errorMessage: "Cancelled",
                wasCancelled: true
            )
        }

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

        let parent = url.deletingLastPathComponent()
        let stem = url.deletingPathExtension().lastPathComponent
        var finalURL = parent.appendingPathComponent("\(stem).\(outExt)")
        if finalURL.path == path || fileManager.fileExists(atPath: finalURL.path) {
            finalURL = parent.appendingPathComponent("\(stem)_compressed.\(outExt)")
        }

        do {
            // Place compressed file first so the original stays intact until replace succeeds.
            try fileManager.moveItem(at: tempOut, to: finalURL)
            var resulting: NSURL?
            do {
                try fileManager.trashItem(at: url, resultingItemURL: &resulting)
            } catch {
                try? fileManager.removeItem(at: finalURL)
                return VideoCompressionResult(
                    originalPath: path,
                    outputPath: "",
                    originalBytes: originalSize,
                    compressedBytes: compressedSize,
                    success: false,
                    errorMessage: error.localizedDescription
                )
            }
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

    private func preferredPreset(for asset: AVAsset, quality: VideoQualityPreset) async -> String {
        let compatible = AVAssetExportSession.exportPresets(compatibleWith: asset)

        let candidates: [String]
        switch quality {
        case .smaller:
            candidates = [
                AVAssetExportPresetHEVC1920x1080,
                AVAssetExportPresetMediumQuality,
                AVAssetExportPreset1280x720,
                AVAssetExportPresetLowQuality
            ]
        case .balanced:
            candidates = [
                AVAssetExportPresetHEVCHighestQuality,
                AVAssetExportPreset1920x1080,
                AVAssetExportPresetHEVC1920x1080,
                AVAssetExportPresetMediumQuality
            ]
        case .higherQuality:
            candidates = [
                AVAssetExportPresetHEVCHighestQuality,
                AVAssetExportPresetHEVC3840x2160,
                AVAssetExportPreset3840x2160,
                AVAssetExportPresetHighestQuality,
                AVAssetExportPreset1920x1080
            ]
        }

        for name in candidates where compatible.contains(name) {
            return name
        }
        if compatible.contains(AVAssetExportPresetMediumQuality) {
            return AVAssetExportPresetMediumQuality
        }
        return AVAssetExportPresetPassthrough
    }
}
