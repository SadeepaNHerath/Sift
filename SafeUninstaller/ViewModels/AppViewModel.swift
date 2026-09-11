import Foundation
import SwiftUI
import UniformTypeIdentifiers
import UninstallerCore
import AppKit

@MainActor
final class AppViewModel: ObservableObject {
    enum SidebarItem: String, CaseIterable, Identifiable {
        case applications = "Applications"
        case leftovers = "Leftovers"
        case history = "History"

        var id: String { rawValue }

        var systemImage: String {
            switch self {
            case .applications: return "square.grid.2x2"
            case .leftovers: return "doc.badge.questionmark"
            case .history: return "clock"
            }
        }
    }

    enum ConfirmMode {
        case uninstall
        case orphans
    }

    @Published var sidebar: SidebarItem = .applications
    @Published var apps: [AppIdentity] = []
    @Published var selectedAppID: String?
    @Published var analysis: AttributionResult?
    @Published var selectionOverrides: [String: Bool] = [:]
    @Published var orphans: [OrphanedResource] = []
    @Published var selectedOrphanID: String?
    @Published var hasScannedOrphans = false
    @Published var reports: [AuditReport] = []
    @Published var selectedReportID: UUID?

    @Published var isListingApps = false
    @Published var isAnalyzing = false
    @Published var isScanningOrphans = false
    @Published var isRemoving = false

    @Published var statusMessage: String = ""
    @Published var toastMessage: String?
    @Published var hasFDA = false
    @Published var helperStatus: String = "Unknown"
    @Published var canRegisterHelper = false
    @Published var lastReport: AuditReport?
    @Published var showConfirmSheet = false
    @Published var confirmMode: ConfirmMode = .uninstall
    @Published var errorMessage: String?
    @Published var showSetupSheet = false
    @Published var setupDismissed = false
    @Published var isDropTargeted = false
    @Published var lowConfidenceExpanded = false

    private let service = UninstallService.shared
    private var analyzeTask: Task<Void, Never>?
    private var analyzeGeneration = 0

    private let setupDismissKey = "SafeUninstaller.setupDismissed"

    var selectedApp: AppIdentity? {
        apps.first { $0.id == selectedAppID }
    }

    var selectedOrphan: OrphanedResource? {
        orphans.first { $0.id == selectedOrphanID }
    }

    var isBusy: Bool {
        isListingApps || isAnalyzing || isScanningOrphans || isRemoving
    }

    var needsSetup: Bool {
        !hasFDA && !setupDismissed
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

    var selectedOrphans: [OrphanedResource] {
        orphans.filter(\.isSelected)
    }

    var confirmPreviewItems: [(path: String, confidence: String)] {
        switch confirmMode {
        case .uninstall:
            return displayedResources
                .filter { $0.isSelected && !$0.isDenied }
                .prefix(12)
                .map { ($0.resource.path, $0.confidence.displayName) }
        case .orphans:
            return selectedOrphans
                .prefix(12)
                .map { ($0.resource.path, $0.confidence.displayName) }
        }
    }

    func bootstrap() {
        service.removal.privilegedClient = HelperXPCClient.shared
        setupDismissed = UserDefaults.standard.bool(forKey: setupDismissKey)
        refreshPermissions()
        refreshApps()
        refreshReports()
        if !hasFDA && !setupDismissed {
            showSetupSheet = true
        }
    }

    func refreshPermissions() {
        hasFDA = service.hasFullDiskAccess()
        helperStatus = HelperXPCClient.shared.registrationStatusDescription
        canRegisterHelper = HelperXPCClient.shared.canRegisterHelper
    }

    func dismissSetup(permanently: Bool) {
        showSetupSheet = false
        if permanently {
            setupDismissed = true
            UserDefaults.standard.set(true, forKey: setupDismissKey)
        }
    }

    func refreshApps() {
        isListingApps = true
        statusMessage = "Scanning Applications…"
        Task.detached(priority: .userInitiated) {
            let list = await MainActor.run { self.service }
                .listApplications()
                .filter { !$0.bundleIdentifier.lowercased().hasPrefix("com.apple.") }
            await MainActor.run {
                self.apps = list
                self.isListingApps = false
                self.statusMessage = "\(list.count) applications"
                if self.selectedAppID == nil {
                    self.selectedAppID = list.first?.id
                    self.analyzeSelected()
                }
            }
        }
    }

    func analyzeSelected() {
        guard let app = selectedApp else {
            analysis = nil
            return
        }
        analyzeGeneration += 1
        let generation = analyzeGeneration
        analyzeTask?.cancel()
        isAnalyzing = true
        statusMessage = "Analyzing \(app.name)…"
        selectionOverrides = [:]
        let appsSnapshot = apps
        analyzeTask = Task.detached(priority: .userInitiated) {
            let result = UninstallService.shared.analyze(app: app, installedApps: appsSnapshot)
            await MainActor.run {
                guard generation == self.analyzeGeneration else { return }
                self.analysis = result
                self.isAnalyzing = false
                self.statusMessage = "\(result.resources.count) related resources"
            }
        }
    }

    func analyzeDroppedApp(url: URL) {
        isAnalyzing = true
        statusMessage = "Analyzing dropped app…"
        Task.detached(priority: .userInitiated) {
            guard let result = UninstallService.shared.analyze(appAt: url) else {
                await MainActor.run {
                    self.isAnalyzing = false
                    self.errorMessage = "Could not read application at \(url.path)"
                }
                return
            }
            await MainActor.run {
                if !self.apps.contains(where: { $0.path == result.app.path }) {
                    self.apps.insert(result.app, at: 0)
                }
                self.selectedAppID = result.app.id
                self.analysis = result
                self.selectionOverrides = [:]
                self.sidebar = .applications
                self.isAnalyzing = false
                self.statusMessage = "\(result.resources.count) related resources"
                self.showToast("Ready to review \(result.app.name)")
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
            DispatchQueue.main.async {
                guard let url else {
                    self.errorMessage = "Could not read dropped item"
                    return
                }
                guard url.pathExtension.lowercased() == "app" else {
                    self.errorMessage = "Drop a macOS .app bundle to analyze it"
                    return
                }
                self.analyzeDroppedApp(url: url)
            }
        }
        return true
    }

    func setSelected(_ resource: AttributedResource, selected: Bool) {
        guard !resource.isDenied else { return }
        selectionOverrides[resource.resource.path] = selected
    }

    func selectAll(confidence: ConfidenceLevel, selected: Bool) {
        for item in displayedResources where item.confidence == confidence && !item.isDenied {
            selectionOverrides[item.resource.path] = selected
        }
    }

    func selectOnlyHighConfidence() {
        for item in displayedResources where !item.isDenied {
            selectionOverrides[item.resource.path] = (item.confidence == .high)
        }
    }

    func requestUninstall() {
        guard selectedCount > 0 else { return }
        confirmMode = .uninstall
        showConfirmSheet = true
    }

    func requestOrphanRemoval() {
        guard !selectedOrphans.isEmpty else { return }
        confirmMode = .orphans
        showConfirmSheet = true
    }

    func performConfirmedAction(dryRun: Bool = false) {
        showConfirmSheet = false
        switch confirmMode {
        case .uninstall:
            performUninstall(dryRun: dryRun)
        case .orphans:
            removeSelectedOrphans()
        }
    }

    func performUninstall(dryRun: Bool = false) {
        guard let analysis else { return }
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
                self.selectedReportID = report.id
                self.isRemoving = false
                self.refreshReports()
                if dryRun {
                    self.showToast("Dry run complete — nothing moved")
                    self.statusMessage = "Dry run: \(report.removedCount) would be removed"
                } else {
                    self.showToast("Moved \(report.removedCount) item(s) to Trash")
                    self.statusMessage = "Removed \(report.removedCount) item(s)"
                    // Refresh inventory; clear analysis if app gone
                    self.refreshApps()
                    if !FileManager.default.fileExists(atPath: analysis.app.path) {
                        self.analysis = nil
                        self.selectedAppID = nil
                    } else {
                        self.analyzeSelected()
                    }
                }
            }
        }
    }

    func scanOrphans() {
        isScanningOrphans = true
        statusMessage = "Searching for leftovers…"
        let appsSnapshot = apps
        Task.detached(priority: .userInitiated) {
            let found = UninstallService.shared.findOrphanedFiles(installedApps: appsSnapshot)
            await MainActor.run {
                self.orphans = found
                self.hasScannedOrphans = true
                self.selectedOrphanID = found.first?.id
                self.isScanningOrphans = false
                self.statusMessage = "\(found.count) leftover candidates"
            }
        }
    }

    func toggleOrphan(_ orphan: OrphanedResource, selected: Bool) {
        guard let idx = orphans.firstIndex(where: { $0.id == orphan.id }) else { return }
        orphans[idx].isSelected = selected
    }

    func removeSelectedOrphans() {
        let selected = selectedOrphans
        guard !selected.isEmpty else { return }
        isRemoving = true
        Task {
            let report = await service.removal.removeOrphans(selected, dryRun: false)
            await MainActor.run {
                self.lastReport = report
                self.selectedReportID = report.id
                self.isRemoving = false
                self.refreshReports()
                self.showToast("Moved \(report.removedCount) leftover(s) to Trash")
                self.scanOrphans()
            }
        }
    }

    func refreshReports() {
        reports = service.removal.loadReports()
        if let id = selectedReportID {
            lastReport = reports.first { $0.id == id } ?? lastReport
        }
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

    func openTrash() {
        let trash = (NSHomeDirectory() as NSString).appendingPathComponent(".Trash")
        NSWorkspace.shared.open(URL(fileURLWithPath: trash))
    }

    func revealInFinder(_ path: String) {
        NSWorkspace.shared.activateFileViewerSelecting([URL(fileURLWithPath: path)])
    }

    func registerHelper() {
        do {
            try HelperXPCClient.shared.register()
            showToast("Helper registration requested")
        } catch {
            errorMessage = error.localizedDescription
        }
        refreshPermissions()
    }

    func showToast(_ message: String) {
        toastMessage = message
        Task {
            try? await Task.sleep(nanoseconds: 4_000_000_000)
            if toastMessage == message {
                toastMessage = nil
            }
        }
    }

    func icon(for app: AppIdentity) -> NSImage {
        NSWorkspace.shared.icon(forFile: app.path)
    }
}
