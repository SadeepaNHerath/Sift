import Foundation

/// Runs official local CLI cleanups when the binary is present (brew / npm / simctl).
public final class CLICleanupRunner: @unchecked Sendable {
    public static let shared = CLICleanupRunner()

    public enum ToolID: String, Sendable, CaseIterable {
        case brew
        case npm
        case simctl
    }

    private let fileManager: FileManager
    private let pathResolver: (String) -> String?

    public init(
        fileManager: FileManager = .default,
        pathResolver: @escaping (String) -> String? = { name in
            CLICleanupRunner.which(name)
        }
    ) {
        self.fileManager = fileManager
        self.pathResolver = pathResolver
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
            // Prefer xcrun when available; fall back to simctl path.
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

        let outPipe = Pipe()
        let errPipe = Pipe()
        process.standardOutput = outPipe
        process.standardError = errPipe

        do {
            try process.run()
            process.waitUntilExit()
        } catch {
            return CLICleanupResult(
                toolID: toolID,
                success: false,
                summary: error.localizedDescription
            )
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

    public static func which(_ name: String) -> String? {
        let process = Process()
        process.executableURL = URL(fileURLWithPath: "/usr/bin/which")
        process.arguments = [name]
        let pipe = Pipe()
        process.standardOutput = pipe
        process.standardError = Pipe()
        do {
            try process.run()
            process.waitUntilExit()
        } catch {
            return nil
        }
        guard process.terminationStatus == 0 else { return nil }
        let data = pipe.fileHandleForReading.readDataToEndOfFile()
        let path = String(data: data, encoding: .utf8)?
            .trimmingCharacters(in: .whitespacesAndNewlines)
        guard let path, !path.isEmpty, FileManager.default.isExecutableFile(atPath: path) else {
            return nil
        }
        return path
    }
}
