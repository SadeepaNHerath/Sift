#!/usr/bin/env swift
import Foundation

// Smoke test: load built framework and analyze a real app (no deletion).
import UninstallerCore

let service = UninstallService.shared
let apps = service.listApplications()
print("Installed third-party-visible apps scanned: \(apps.count)")

guard let target = apps.first(where: { $0.name.localizedCaseInsensitiveContains("LocalSend") })
        ?? apps.first(where: { !$0.bundleIdentifier.lowercased().hasPrefix("com.apple.") }) else {
    print("No suitable app found")
    exit(1)
}

print("Analyzing: \(target.name) [\(target.bundleIdentifier)]")
print("Path: \(target.path)")
print("Team: \(target.teamID ?? "nil")")

let result = service.analyze(app: target, installedApps: apps)
print("Resources found: \(result.resources.count)")
for level in [ConfidenceLevel.high, .medium, .low] {
    let items = result.resources(with: level)
    print("  \(level.displayName): \(items.count)")
    for item in items.prefix(5) {
        print("    - \(item.resource.displayName) [\(item.evidenceSummary)] \(item.resource.formattedSize)")
    }
}
let denied = result.resources.filter(\.isDenied)
print("  Denied: \(denied.count)")

print("FDA likely: \(service.hasFullDiskAccess())")
print("SMOKE_OK")
