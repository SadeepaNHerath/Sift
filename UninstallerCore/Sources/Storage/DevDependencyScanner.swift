import Foundation

/// Scans Xcode / package caches and per-project dependency folders. Nothing is pre-selected.
public final class DevDependencyScanner: @unchecked Sendable {
    public static let shared = DevDependencyScanner()

    public static let projectDependencyNames: Set<String> = [
        "node_modules", "Pods", ".venv", "venv", "target", ".gradle", "build"
    ]

    public static let manifests: [String: String] = [
        "node_modules": "package.json",
        "Pods": "Podfile",
        ".venv": "pyproject.toml",
        "venv": "requirements.txt",
        "target": "Cargo.toml",
        ".gradle": "build.gradle",
        "build": "build.gradle"
    ]

    private let fileManager: FileManager
    private let denylist: SafetyDenylist
    private let overview: StorageOverviewEngine

    public init(
        fileManager: FileManager = .default,
        denylist: SafetyDenylist = .shared,
        overview: StorageOverviewEngine = .shared
    ) {
        self.fileManager = fileManager
        self.denylist = denylist
        self.overview = overview
    }

    public func scan(
        home: URL? = nil,
        projectRoots: [URL]? = nil,
        maxProjectDepth: Int = 4
    ) -> [DevDependencyItem] {
        let homeURL = home ?? fileManager.homeDirectoryForCurrentUser
        var items: [DevDependencyItem] = []
        items.append(contentsOf: scanXcode(home: homeURL))
        items.append(contentsOf: scanPackageCaches(home: homeURL))
        items.append(contentsOf: scanProjectFolders(
            roots: projectRoots ?? defaultProjectRoots(home: homeURL),
            maxDepth: maxProjectDepth
        ))
        return items.sorted { $0.byteSize > $1.byteSize }
    }

    public func defaultProjectRoots(home: URL) -> [URL] {
        [
            home.appendingPathComponent("Documents"),
            home.appendingPathComponent("Developer"),
            home.appendingPathComponent("Projects"),
            home.appendingPathComponent("Desktop"),
            home.appendingPathComponent("Source")
        ]
    }

    private func scanXcode(home: URL) -> [DevDependencyItem] {
        let pairs: [(String, String)] = [
            ("Library/Developer/Xcode/DerivedData", "Rebuilds next Xcode build. Safe if no builds are in progress."),
            ("Library/Developer/Xcode/Archives", "Removes archived builds used for App Store / TestFlight uploads."),
            ("Library/Developer/Xcode/iOS DeviceSupport", "Symbols for connected devices; re-downloads when you plug a device in."),
            ("Library/Developer/CoreSimulator/Caches", "Simulator caches; simulators will cold-start slower next time.")
        ]
        return pairs.compactMap { relative, note in
            let url = home.appendingPathComponent(relative)
            guard fileManager.fileExists(atPath: url.path) else { return nil }
            if denylist.isDenied(path: url.path) { return nil }
            let size = overview.sizeOfDirectory(at: url, maxItems: 4_000)
            guard size > 0 else { return nil }
            return DevDependencyItem(
                path: url.path,
                byteSize: size,
                group: .xcode,
                consequence: note,
                isHighRisk: relative.contains("Archives"),
                isSelected: false
            )
        }
    }

    private func scanPackageCaches(home: URL) -> [DevDependencyItem] {
        let pairs: [(String, String, Bool)] = [
            (".npm", "npm package cache — re-downloaded on next install.", false),
            (".yarn", "Yarn cache — safe to clear.", false),
            (".pnpm-store", "pnpm content-addressable store — projects re-link on next install.", false),
            (".gradle/caches", "Gradle caches — next Android/build will re-download.", false),
            (".m2/repository", "Maven local repository — re-downloaded as needed.", false),
            (".cocoapods", "CocoaPods cache — pods re-fetched on next pod install.", false),
            (".cargo/registry", "Rust crate cache — cargo will re-fetch.", false),
            ("Library/Caches/Homebrew", "Homebrew downloads cache — safe to clear.", false),
            ("Library/Caches/pip", "pip wheel cache — safe to clear.", false),
            (".docker", "Docker data (images/volumes). High risk — only if you use Docker and accept wiping local images.", true)
        ]
        return pairs.compactMap { relative, note, highRisk in
            let url = home.appendingPathComponent(relative)
            guard fileManager.fileExists(atPath: url.path) else { return nil }
            if denylist.isDenied(path: url.path) { return nil }
            let size = overview.sizeOfDirectory(at: url, maxItems: 4_000)
            guard size > 0 else { return nil }
            return DevDependencyItem(
                path: url.path,
                byteSize: size,
                group: .packageCaches,
                consequence: note,
                isHighRisk: highRisk,
                isSelected: false
            )
        }
    }

    /// Only flags dependency folders that sit next to their expected manifest.
    public func scanProjectFolders(roots: [URL], maxDepth: Int) -> [DevDependencyItem] {
        var items: [DevDependencyItem] = []
        var seen = Set<String>()

        for root in roots {
            guard fileManager.fileExists(atPath: root.path) else { continue }
            walk(root: root, depth: 0, maxDepth: maxDepth, items: &items, seen: &seen)
        }
        return items
    }

    /// Returns true when `folderName` at `parent` is a valid project dependency (manifest present).
    public func isManifestAdjacentDependency(parent: URL, folderName: String) -> Bool {
        guard Self.projectDependencyNames.contains(folderName) else { return false }
        let dependencyURL = parent.appendingPathComponent(folderName)
        guard fileManager.fileExists(atPath: dependencyURL.path) else { return false }

        // Prefer exact mapped manifest; also accept common alternates.
        if let manifest = Self.manifests[folderName] {
            if fileManager.fileExists(atPath: parent.appendingPathComponent(manifest).path) {
                return true
            }
        }
        // Alternates
        switch folderName {
        case "node_modules":
            return fileManager.fileExists(atPath: parent.appendingPathComponent("package.json").path)
        case "Pods":
            return fileManager.fileExists(atPath: parent.appendingPathComponent("Podfile").path)
                || fileManager.fileExists(atPath: parent.appendingPathComponent("Podfile.lock").path)
        case ".venv", "venv":
            return fileManager.fileExists(atPath: parent.appendingPathComponent("pyproject.toml").path)
                || fileManager.fileExists(atPath: parent.appendingPathComponent("requirements.txt").path)
                || fileManager.fileExists(atPath: parent.appendingPathComponent("Pipfile").path)
        case "target":
            return fileManager.fileExists(atPath: parent.appendingPathComponent("Cargo.toml").path)
        case ".gradle", "build":
            return fileManager.fileExists(atPath: parent.appendingPathComponent("build.gradle").path)
                || fileManager.fileExists(atPath: parent.appendingPathComponent("build.gradle.kts").path)
                || fileManager.fileExists(atPath: parent.appendingPathComponent("settings.gradle").path)
        default:
            return false
        }
    }

    private func walk(
        root: URL,
        depth: Int,
        maxDepth: Int,
        items: inout [DevDependencyItem],
        seen: inout Set<String>
    ) {
        guard depth <= maxDepth else { return }
        let children: [URL]
        do {
            children = try fileManager.contentsOfDirectory(
                at: root,
                includingPropertiesForKeys: [.isDirectoryKey],
                options: [.skipsHiddenFiles]
            )
        } catch {
            return
        }

        for child in children {
            var isDir: ObjCBool = false
            guard fileManager.fileExists(atPath: child.path, isDirectory: &isDir), isDir.boolValue else {
                continue
            }
            let name = child.lastPathComponent

            if isManifestAdjacentDependency(parent: root, folderName: name) {
                if denylist.isDenied(path: child.path) { continue }
                guard seen.insert(child.path).inserted else { continue }
                let size = overview.sizeOfDirectory(at: child, maxItems: 3_000)
                guard size > 0 else { continue }
                let note: String
                switch name {
                case "node_modules":
                    note = "Restore with npm/yarn/pnpm install in this project."
                case "Pods":
                    note = "Restore with pod install."
                case ".venv", "venv":
                    note = "Recreate the Python virtualenv and reinstall requirements."
                case "target":
                    note = "Rust build output — restore with cargo build."
                default:
                    note = "Project build/dependency folder — reinstall or rebuild as needed."
                }
                items.append(
                    DevDependencyItem(
                        path: child.path,
                        byteSize: size,
                        group: .projectFolders,
                        consequence: note,
                        isHighRisk: false,
                        isSelected: false
                    )
                )
                // Don't walk inside dependency folders
                continue
            }

            // Skip heavy known trees when walking deeper
            if Self.projectDependencyNames.contains(name) { continue }
            if name == "Library" || name == ".git" || name == "DerivedData" { continue }
            walk(root: child, depth: depth + 1, maxDepth: maxDepth, items: &items, seen: &seen)
        }
    }
}
