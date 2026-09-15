import Foundation

/// Shared Process runner with wall-clock timeout (SIGTERM then SIGKILL).
public enum TimedProcess {
    public struct Result: Sendable {
        public let status: Int32
        public let stdout: Data
        public let stderr: Data
        public let timedOut: Bool
    }

    @discardableResult
    public static func run(
        executable: String,
        arguments: [String],
        timeout: TimeInterval = 15,
        environment: [String: String]? = nil
    ) -> Result {
        let process = Process()
        process.executableURL = URL(fileURLWithPath: executable)
        process.arguments = arguments
        if let environment {
            process.environment = environment
        }
        let outPipe = Pipe()
        let errPipe = Pipe()
        process.standardOutput = outPipe
        process.standardError = errPipe

        do {
            try process.run()
        } catch {
            return Result(status: -1, stdout: Data(), stderr: Data(), timedOut: false)
        }

        let deadline = Date().addingTimeInterval(timeout)
        var timedOut = false
        while process.isRunning {
            if Date() >= deadline {
                timedOut = true
                process.terminate()
                let grace = Date().addingTimeInterval(2)
                while process.isRunning && Date() < grace {
                    Thread.sleep(forTimeInterval: 0.05)
                }
                if process.isRunning {
                    kill(process.processIdentifier, SIGKILL)
                }
                break
            }
            Thread.sleep(forTimeInterval: 0.05)
        }

        let stdout = outPipe.fileHandleForReading.readDataToEndOfFile()
        let stderr = errPipe.fileHandleForReading.readDataToEndOfFile()
        return Result(
            status: timedOut ? -9 : process.terminationStatus,
            stdout: stdout,
            stderr: stderr,
            timedOut: timedOut
        )
    }
}
