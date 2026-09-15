import Foundation

/// Runs official local CLI cleanups when the binary is present (brew / npm / simctl).
public final class CLICleanupRunner: @unchecked Sendable {
    public static let shared = CLICleanupRunner()

    public enum ToolID: String, Sendable, CaseIterable {
        case brew
        case npm
        case simctl
    }

    public static let defaultTimeout: TimeInterval = 120

    /// Directories prepended when resolving tools and when spawning Process (Dock-launched PATH fix).
    public static let augmentedPathDirectories: [String] = [
        "/opt/homebrew/bin",
        "/opt/homebrew/sbin",
        "/usr/local/bin",
        "/usr/bin",
        (NSHomeDirectory() as NSString).appendingPathComponent(".local/bin")
    ]

    private let fileManager: FileManager
    private let pathResolver: (String) -> String?
    private let timeout: TimeInterval

    public init(
        fileManager: FileManager = .default,
        timeout: TimeInterval = CLICleanupRunner.defaultTimeout,
        pathResolver: ((String) -> String?)? = nil
    ) {
        self.fileManager = fileManager
        self.timeout = timeout
        self.pathResolver = pathResolver ?? { name in
            CLICleanupRunner.resolveExecutable(named: name, fileManager: fileManager)
        }
    }

    public func availableActions() -> [CLICleanupAction] {
        ToolID.allCases.map { tool in
            let available = resolveExecutable(for: tool) != nil
            switch tool {
            case .brew:
                return CLICleanupAction(
                    toolID: tool.rawValue,
                    title: "Homebrew cleanup",
                    detail: "Runs `brew cleanup -s` to remove old downloads and cache.",
                    isAvailable: available
                )
            case .npm:
                return CLICleanupAction(
                    toolID: tool.rawValue,
                    title: "npm cache clean",
                    detail: "Runs `npm cache clean --force`.",
                    isAvailable: available
                )
            case .simctl:
                return CLICleanupAction(
                    toolID: tool.rawValue,
                    title: "Delete unavailable simulators",
                    detail: "Runs `xcrun simctl delete unavailable`.",
                    isAvailable: available
                )
            }
        }
    }

    public func run(_ toolID: String) -> CLICleanupResult {
        guard let tool = ToolID(rawValue: toolID) else {
            return CLICleanupResult(toolID: toolID, success: false, summary: "Unknown tool")
        }
        return run(tool)
    }

    public func run(_ tool: ToolID) -> CLICleanupResult {
        guard let executable = resolveExecutable(for: tool) else {
            return CLICleanupResult(
                toolID: tool.rawValue,
                success: false,
                summary: "\(tool.rawValue) is not installed or not on PATH"
            )
        }

        let arguments: [String]
        let launchPath: String
        switch tool {
        case .brew:
            launchPath = executable
            arguments = ["cleanup", "-s"]
        case .npm:
            launchPath = executable
            arguments = ["cache", "clean", "--force"]
        case .simctl:
            if let xcrun = pathResolver("xcrun") {
                launchPath = xcrun
                arguments = ["simctl", "delete", "unavailable"]
            } else {
                launchPath = executable
                arguments = ["delete", "unavailable"]
            }
        }

        return execute(toolID: tool.rawValue, launchPath: launchPath, arguments: arguments)
    }

    /// Exposed for tests (timeout + PATH environment).
    public func runProcess(toolID: String, launchPath: String, arguments: [String]) -> CLICleanupResult {
        execute(toolID: toolID, launchPath: launchPath, arguments: arguments)
    }

    private func resolveExecutable(for tool: ToolID) -> String? {
        switch tool {
        case .brew:
            return pathResolver("brew")
        case .npm:
            return pathResolver("npm")
        case .simctl:
            if pathResolver("xcrun") != nil { return pathResolver("simctl") ?? "/usr/bin/simctl" }
            return pathResolver("simctl")
        }
    }

    private func execute(toolID: String, launchPath: String, arguments: [String]) -> CLICleanupResult {
        let process = Process()
        process.executableURL = URL(fileURLWithPath: launchPath)
        process.arguments = arguments

        var env = ProcessInfo.processInfo.environment
        let inherited = env["PATH"] ?? "/usr/bin:/bin"
        let prefix = Self.augmentedPathDirectories.joined(separator: ":")
        env["PATH"] = prefix + ":" + inherited
        process.environment = env

        let outPipe = Pipe()
        let errPipe = Pipe()
        process.standardOutput = outPipe
        process.standardError = errPipe

        do {
            try process.run()
        } catch {
            return CLICleanupResult(
                toolID: toolID,
                success: false,
                summary: error.localizedDescription
            )
        }

        let deadline = Date().addingTimeInterval(timeout)
        while process.isRunning {
            if Date() >= deadline {
                process.terminate()
                // Give it a moment to exit after SIGTERM.
                let grace = Date().addingTimeInterval(2)
                while process.isRunning && Date() < grace {
                    Thread.sleep(forTimeInterval: 0.05)
                }
                if process.isRunning {
                    kill(process.processIdentifier, SIGKILL)
                }
                return CLICleanupResult(
                    toolID: toolID,
                    success: false,
                    summary: "Timed out after \(Int(timeout))s"
                )
            }
            Thread.sleep(forTimeInterval: 0.05)
        }

        let stdout = String(data: outPipe.fileHandleForReading.readDataToEndOfFile(), encoding: .utf8)?
            .trimmingCharacters(in: .whitespacesAndNewlines) ?? ""
        let stderr = String(data: errPipe.fileHandleForReading.readDataToEndOfFile(), encoding: .utf8)?
            .trimmingCharacters(in: .whitespacesAndNewlines) ?? ""

        let combined = [stdout, stderr].filter { !$0.isEmpty }.joined(separator: "\n")
        let success = process.terminationStatus == 0
        let summary: String
        if combined.isEmpty {
            summary = success ? "Completed successfully." : "Exited with status \(process.terminationStatus)"
        } else {
            let clipped = combined.count > 800 ? String(combined.prefix(800)) + "…" : combined
            summary = clipped
        }
        return CLICleanupResult(toolID: toolID, success: success, summary: summary)
    }

    /// Resolve an executable from well-known dirs, then `which`.
    public static func resolveExecutable(named name: String, fileManager: FileManager = .default) -> String? {
        for dir in augmentedPathDirectories {
            let candidate = (dir as NSString).appendingPathComponent(name)
            if fileManager.isExecutableFile(atPath: candidate) {
                return candidate
            }
        }
        return which(name)
    }

    public static func which(_ name: String) -> String? {
        var env = ProcessInfo.processInfo.environment
        let inherited = env["PATH"] ?? "/usr/bin:/bin"
        env["PATH"] = augmentedPathDirectories.joined(separator: ":") + ":" + inherited
        let result = TimedProcess.run(
            executable: "/usr/bin/which",
            arguments: [name],
            timeout: 5,
            environment: env
        )
        guard !result.timedOut, result.status == 0 else { return nil }
        let path = String(data: result.stdout, encoding: .utf8)?
            .trimmingCharacters(in: .whitespacesAndNewlines)
        guard let path, !path.isEmpty, FileManager.default.isExecutableFile(atPath: path) else {
            return nil
        }
        return path
    }
}
