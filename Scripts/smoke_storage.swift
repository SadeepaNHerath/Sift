#!/usr/bin/env swift
import Foundation
import UninstallerCore

let service = UninstallService.shared

print("=== Storage Overview (dry) ===")
let overview = service.storageSnapshot()
print("Free: \(overview.formattedAvailable) / \(overview.formattedTotal) (\(Int(overview.usedFraction * 100))% used)")
for cat in overview.categories {
    print("  \(cat.kind.displayName): \(cat.formattedSize)")
}

print("\n=== Large Files (>=100MB, dry) ===")
let large = service.scanLargeFiles()
print("Count: \(large.count)")
for item in large.prefix(5) {
    print("  \(item.formattedSize)  \(item.isVideo ? "[video]" : "[file]")  \(item.displayName)")
}

print("\n=== Caches (dry) ===")
let caches = service.scanCaches()
print("Count: \(caches.count)")
for item in caches.prefix(5) {
    print("  \(item.formattedSize)  \(item.displayName)")
}

print("\n=== Developer deps (dry) ===")
let dev = service.scanDevDependencies()
print("Count: \(dev.count)")
for item in dev.prefix(8) {
    print("  \(item.formattedSize)  [\(item.group.displayName)]  \(item.displayName)")
}

print("\nSMOKE_STORAGE_OK")
