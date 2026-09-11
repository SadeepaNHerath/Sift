import Foundation
import SwiftUI
import UniformTypeIdentifiers
import UninstallerCore
import AppKit

@MainActor
final class AppViewModel: ObservableObject {
    enum SidebarItem: String, CaseIterable, Identifiable {
        case applications = "Applications"
        case orphans = "Orphaned Files"
        case reports = "Reports"
        case settings = "Permissions"

        var id: String { rawValue }

        var systemImage: String {
            switch self {
            case .applications: return "square.grid.2x2"
            case .orphans: return "doc.badge.questionmark"
            case .reports: return "doc.text.magnifyingglass"
            case .settings: return "lock.shield"
            }
        }
    }

    @Published var sidebar: SidebarItem = .applications
    @Published var apps: [AppIdentity] = []
    @Published var selectedAppID: String?
    @Published var analysis: AttributionResult?
    @Published var selectionOverrides: [String: Bool] = [:]
    @Published var orphans: [OrphanedResource] = []
    @Published var reports: [AuditReport] = []
    @Published var isScanning = false
    @Published var isRemoving = false
    @Published var statusMessage: String = ""
    @Published var hasFDA = false
    @Published var helperStatus: String = "Unknown"
    @Published var lastReport: AuditReport?
    @Published var showConfirmSheet = false
    @Published var errorMessage: String?

    private let service = UninstallService.shared

    var selectedApp: AppIdentity? {
        apps.first { $0.id == selectedAppID }
    }

    var displayedResources: [AttributedResource] {
        guard let analysis else { return [] }
        return analysis.resources.map { item in
            var copy = item
            if let override = selectionOverrides[item.resource.path] {
                copy.isSelected = override
            }
            return copy
        }
    }

    var selectedCount: Int {
        displayedResources.filter { $0.isSelected && !$0.isDenied }.count
    }

    var selectedBytes: Int64 {
        displayedResources.filter { $0.isSelected && !$0.isDenied }.reduce(0) { $0 + $1.resource.byteSize }
    }

    func bootstrap() {
        service.removal.privilegedClient = HelperXPCClient.shared
        refreshPermissions()
        refreshApps()
        refreshReports()
    }

    func refreshPermissions() {
        hasFDA = service.hasFullDiskAccess()
        helperStatus = HelperXPCClient.shared.registrationStatusDescription
    }

    func refreshApps() {
        isScanning = true
        statusMessage = "Scanning Applications…"
        DispatchQueue.global(qos: .userInitiated).async {
            let list = self.service.listApplications()
                .filter { !$0.bundleIdentifier.lowercased().hasPrefix("com.apple.") }
            DispatchQueue.main.async {
                self.apps = list
                self.isScanning = false
                self.statusMessage = "\(list.count) applications found"
                if self.selectedAppID == nil {
                    self.selectedAppID = list.first?.id
                }
            }
        }
    }

    func analyzeSelected() {
        guard let app = selectedApp else { return }
        isScanning = true
        statusMessage = "Analyzing \(app.name)…"
        selectionOverrides = [:]
        DispatchQueue.global(qos: .userInitiated).async {
            let result = self.service.analyze(app: app, installedApps: self.apps)
            DispatchQueue.main.async {
                self.analysis = result
                self.isScanning = false
                self.statusMessage = "Found \(result.resources.count) related resources"
            }
        }
    }

    func analyzeDroppedApp(url: URL) {
        isScanning = true
        statusMessage = "Analyzing dropped app…"
        DispatchQueue.global(qos: .userInitiated).async {
            guard let result = self.service.analyze(appAt: url) else {
                DispatchQueue.main.async {
                    self.isScanning = false
                    self.errorMessage = "Could not read application at \(url.path)"
                }
                return
            }
            DispatchQueue.main.async {
                if !self.apps.contains(where: { $0.path == result.app.path }) {
                    self.apps.insert(result.app, at: 0)
                }
                self.selectedAppID = result.app.id
                self.analysis = result
                self.selectionOverrides = [:]
                self.sidebar = .applications
                self.isScanning = false
                self.statusMessage = "Found \(result.resources.count) related resources"
            }
        }
    }

    func handleDrop(providers: [NSItemProvider]) -> Bool {
        guard let provider = providers.first else { return false }
        provider.loadItem(forTypeIdentifier: UTType.fileURL.identifier, options: nil) { item, _ in
            var url: URL?
            if let data = item as? Data {
                url = URL(dataRepresentation: data, relativeTo: nil)
            } else if let str = item as? NSString {
                url = URL(fileURLWithPath: str as String)
            } else if let u = item as? URL {
                url = u
            }
            guard let url, url.pathExtension.lowercased() == "app" else { return }
            DispatchQueue.main.async {
                self.analyzeDroppedApp(url: url)
            }
        }
        return true
    }

    func setSelected(_ resource: AttributedResource, selected: Bool) {
        guard !resource.isDenied else { return }
        selectionOverrides[resource.resource.path] = selected
    }

    func requestUninstall() {
        guard selectedCount > 0 else { return }
        showConfirmSheet = true
    }

    func performUninstall(dryRun: Bool = false) {
        guard let analysis else { return }
        showConfirmSheet = false
        isRemoving = true
        statusMessage = dryRun ? "Dry run…" : "Moving items to Trash…"
        let overrides = selectionOverrides
        Task {
            let report = await service.uninstall(
                result: analysis,
                selectedOverrides: overrides.isEmpty ? nil : overrides,
                dryRun: dryRun
            )
            await MainActor.run {
                self.lastReport = report
                self.isRemoving = false
                self.statusMessage = "Removed \(report.removedCount) item(s)"
                self.refreshReports()
                if !dryRun {
                    self.analyzeSelected()
                    self.refreshApps()
                }
                self.sidebar = .reports
            }
        }
    }

    func scanOrphans() {
        isScanning = true
        statusMessage = "Searching for orphaned files…"
        DispatchQueue.global(qos: .userInitiated).async {
            let found = self.service.findOrphanedFiles(installedApps: self.apps)
            DispatchQueue.main.async {
                self.orphans = found
                self.isScanning = false
                self.statusMessage = "\(found.count) orphaned candidates"
            }
        }
    }

    func toggleOrphan(_ orphan: OrphanedResource, selected: Bool) {
        guard let idx = orphans.firstIndex(where: { $0.id == orphan.id }) else { return }
        orphans[idx].isSelected = selected
    }

    func removeSelectedOrphans() {
        let selected = orphans.filter(\.isSelected)
        guard !selected.isEmpty else { return }
        isRemoving = true
        Task {
            let report = await service.removal.removeOrphans(selected, dryRun: false)
            await MainActor.run {
                self.lastReport = report
                self.isRemoving = false
                self.refreshReports()
                self.scanOrphans()
                self.sidebar = .reports
            }
        }
    }

    func refreshReports() {
        reports = service.removal.loadReports()
    }

    func openFullDiskAccessSettings() {
        if let url = URL(string: "x-apple.systempreferences:com.apple.preference.security?Privacy_AllFiles") {
            NSWorkspace.shared.open(url)
        }
    }

    func openLoginItemsSettings() {
        if let url = URL(string: "x-apple.systempreferences:com.apple.LoginItems-Settings.extension") {
            NSWorkspace.shared.open(url)
        }
    }

    func registerHelper() {
        do {
            try HelperXPCClient.shared.register()
            statusMessage = "Helper registration requested"
        } catch {
            errorMessage = error.localizedDescription
        }
        refreshPermissions()
    }
}
