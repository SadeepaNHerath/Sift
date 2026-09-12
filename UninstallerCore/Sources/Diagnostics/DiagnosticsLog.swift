import Foundation

/// On-device append-only diagnostics log (no network / no telemetry).
public final class DiagnosticsLog: @unchecked Sendable {
    public static let shared = DiagnosticsLog()

    private let queue = DispatchQueue(label: "com.safeuninstaller.diagnostics")
    private let fileManager: FileManager
    private let maxBytes: Int
    private let logURL: URL

    public init(fileManager: FileManager = .default, maxBytes: Int = 1_000_000) {
        self.fileManager = fileManager
        self.maxBytes = maxBytes
        let logs = fileManager.homeDirectoryForCurrentUser
            .appendingPathComponent("Library/Logs/SafeUninstaller", isDirectory: true)
        try? fileManager.createDirectory(at: logs, withIntermediateDirectories: true)
        self.logURL = logs.appendingPathComponent("app.log")
    }

    public var directoryURL: URL { logURL.deletingLastPathComponent() }
    public var fileURL: URL { logURL }

    public func info(_ message: String) { append(level: "INFO", message: message) }
    public func error(_ message: String) { append(level: "ERROR", message: message) }

    public func append(level: String, message: String) {
        let line = "\(isoNow()) [\(level)] \(message)\n"
        queue.async {
            self.rotateIfNeeded()
            if let data = line.data(using: .utf8) {
                if self.fileManager.fileExists(atPath: self.logURL.path) {
                    if let handle = try? FileHandle(forWritingTo: self.logURL) {
                        defer { try? handle.close() }
                        _ = try? handle.seekToEnd()
                        try? handle.write(contentsOf: data)
                    }
                } else {
                    try? data.write(to: self.logURL, options: .atomic)
                }
            }
        }
    }

    /// Last N lines for Copy Diagnostics (synchronous read).
    public func tailLines(_ maxLines: Int = 200) -> String {
        queue.sync {
            guard let data = try? Data(contentsOf: logURL),
                  let text = String(data: data, encoding: .utf8),
                  !text.isEmpty else {
                return "(no log entries yet)"
            }
            let lines = text.split(separator: "\n", omittingEmptySubsequences: false)
            let slice = lines.suffix(maxLines)
            return slice.joined(separator: "\n")
        }
    }

    public func installExceptionHook() {
        NSSetUncaughtExceptionHandler { exception in
            let reason = exception.reason ?? "(no reason)"
            let name = exception.name.rawValue
            let stack = exception.callStackSymbols.prefix(40).joined(separator: "\n")
            DiagnosticsLog.shared.error("Uncaught \(name): \(reason)\n\(stack)")
        }
    }

    private func rotateIfNeeded() {
        guard let attrs = try? fileManager.attributesOfItem(atPath: logURL.path),
              let size = attrs[.size] as? NSNumber,
              size.intValue >= maxBytes else { return }
        let backup = logURL.deletingPathExtension().appendingPathExtension("log.1")
        try? fileManager.removeItem(at: backup)
        try? fileManager.moveItem(at: logURL, to: backup)
    }

    private func isoNow() -> String {
        ISO8601DateFormatter().string(from: Date())
    }
}
