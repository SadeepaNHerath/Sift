import Foundation
import SwiftUI
import UniformTypeIdentifiers
import UninstallerCore
import AppKit

@MainActor
final class AppViewModel: ObservableObject {
    enum SidebarItem: String, CaseIterable, Identifiable {
        case overview = "Overview"
        case applications = "Applications"
        case leftovers = "Leftovers"
        case largeFiles = "Large Files"
        case caches = "Caches"
        case developer = "Developer"
        case history = "History"

        var id: String { rawValue }

        var systemImage: String {
            switch self {
            case .overview: return "gauge.with.dots.needle.33percent"
            case .applications: return "square.grid.2x2"
            case .leftovers: return "doc.badge.questionmark"
            case .largeFiles: return "film.stack"
            case .caches: return "internaldrive"
            case .developer: return "hammer"
            case .history: return "clock"
            }
        }
    }

    enum ConfirmMode {
        case uninstall
        case orphans
        case largeFiles
        case caches
        case developer
    }

    @Published var sidebar: SidebarItem = .overview
    @Published var apps: [AppIdentity] = []
    @Published var selectedAppID: String?
    @Published var analysis: AttributionResult?
    @Published var selectionOverrides: [String: Bool] = [:]
    @Published var orphans: [OrphanedResource] = []
    @Published var selectedOrphanID: String?
    @Published var hasScannedOrphans = false
    @Published var reports: [AuditReport] = []
    @Published var selectedReportID: UUID?

    // Storage toolkit
    @Published var storageOverview: StorageOverview?
    @Published var isLoadingOverview = false
    @Published var largeFileItems: [LargeFileItem] = []
    @Published var hasScannedLargeFiles = false
    @Published var isScanningLargeFiles = false
    @Published var cacheItems: [CacheItem] = []
    @Published var hasScannedCaches = false
    @Published var isScanningCaches = false
    @Published var devItems: [DevDependencyItem] = []
    @Published var hasScannedDev = false
    @Published var isScanningDev = false
    @Published var compressingPath: String?
    @Published var compressionProgress: Double = 0
    @Published var videoQualityPreset: VideoQualityPreset = .balanced
    @Published var imageOutputFormat: ImageOutputFormat = .automatic
    @Published var lastCompressionSavings: String?
    @Published var compressionEstimateText: String?
    @Published var cliCleanupActions: [CLICleanupAction] = []
    @Published var pendingCLIAction: CLICleanupAction?
    @Published var showCLICleanupConfirm = false
    @Published var isRunningCLICleanup = false
    @Published var cliCleanupSummary: String?
    @Published var showEmptyTrashConfirm = false
    private var compressionTask: Task<Void, Never>?

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
            || isLoadingOverview || isScanningLargeFiles || isScanningCaches
            || isScanningDev || compressingPath != nil || isRunningCLICleanup
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

    var selectedLargeFiles: [LargeFileItem] {
        largeFileItems.filter(\.isSelected)
    }

    var selectedCaches: [CacheItem] {
        cacheItems.filter(\.isSelected)
    }

    var selectedDevItems: [DevDependencyItem] {
        devItems.filter(\.isSelected)
    }

    var trashBytes: Int64 {
        storageOverview?.categories.first(where: { $0.kind == .trash })?.byteSize ?? 0
    }

    var confirmPreviewItems: [(path: String, confidence: String)] {
        switch confirmMode {
        case .uninstall:
            return displayedResources
                .filter { $0.isSelected && !$0.isDenied }
                .prefix(12)
                .map { ($0.resource.path, $0.confidence.displayName) }
        case .orphans:
            return selectedOrphans.prefix(12).map { ($0.resource.path, $0.confidence.displayName) }
        case .largeFiles:
            return selectedLargeFiles.prefix(12).map {
                ($0.path, $0.isVideo ? "Video" : ($0.isImage ? "Image" : "File"))
            }
        case .caches:
            return selectedCaches.prefix(12).map { ($0.path, "Cache") }
        case .developer:
            return selectedDevItems.prefix(12).map { ($0.path, $0.group.displayName) }
        }
    }

    var confirmSelectedCount: Int {
        switch confirmMode {
        case .uninstall: return selectedCount
        case .orphans: return selectedOrphans.count
        case .largeFiles: return selectedLargeFiles.count
        case .caches: return selectedCaches.count
        case .developer: return selectedDevItems.count
        }
    }

    var confirmSelectedBytes: Int64 {
        switch confirmMode {
        case .uninstall: return selectedBytes
        case .orphans: return selectedOrphans.reduce(0) { $0 + $1.resource.byteSize }
        case .largeFiles: return selectedLargeFiles.reduce(0) { $0 + $1.byteSize }
        case .caches: return selectedCaches.reduce(0) { $0 + $1.byteSize }
        case .developer: return selectedDevItems.reduce(0) { $0 + $1.byteSize }
        }
    }

    func bootstrap() {
        service.removal.privilegedClient = HelperXPCClient.shared
        setupDismissed = UserDefaults.standard.bool(forKey: setupDismissKey)
        refreshPermissions()
        refreshApps()
        refreshReports()
        refreshStorageOverview()
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

    // MARK: - Storage overview

    func refreshStorageOverview() {
        isLoadingOverview = true
        statusMessage = "Measuring storage…"
        Task.detached(priority: .userInitiated) {
            let snap = UninstallService.shared.storageSnapshot()
            await MainActor.run {
                self.storageOverview = snap
                self.isLoadingOverview = false
                self.statusMessage = "\(snap.formattedAvailable) free of \(snap.formattedTotal)"
            }
        }
    }

    func requestEmptyTrash() {
        showEmptyTrashConfirm = true
    }

    func emptyTrash() {
        showEmptyTrashConfirm = false
        if let err = service.removal.emptyTrash() {
            errorMessage = err
        } else {
            showToast("Trash emptied")
            refreshStorageOverview()
        }
    }

    // MARK: - Large files & video

    func scanLargeFiles() {
        isScanningLargeFiles = true
        statusMessage = "Scanning large files…"
        Task.detached(priority: .userInitiated) {
            let found = UninstallService.shared.scanLargeFiles()
            await MainActor.run {
                self.largeFileItems = found
                self.hasScannedLargeFiles = true
                self.isScanningLargeFiles = false
                self.statusMessage = "\(found.count) large files"
            }
        }
    }

    func toggleLargeFile(_ item: LargeFileItem, selected: Bool) {
        guard let idx = largeFileItems.firstIndex(where: { $0.id == item.id }) else { return }
        largeFileItems[idx].isSelected = selected
    }

    func requestLargeFileRemoval() {
        guard !selectedLargeFiles.isEmpty else { return }
        confirmMode = .largeFiles
        showConfirmSheet = true
    }

    func removeSelectedLargeFiles() {
        let selected = selectedLargeFiles
        guard !selected.isEmpty else { return }
        isRemoving = true
        let paths = selected.map { (path: $0.path, category: ResourceCategory.largeFile, byteSize: $0.byteSize) }
        Task {
            let report = await service.removal.trashPaths(paths, reportName: "Large Files")
            await MainActor.run {
                self.lastReport = report
                self.selectedReportID = report.id
                self.isRemoving = false
                self.refreshReports()
                self.showToast("Moved \(report.removedCount) large file(s) to Trash")
                self.scanLargeFiles()
                self.refreshStorageOverview()
            }
        }
    }

    func compressVideo(_ item: LargeFileItem) {
        guard item.isVideo else { return }
        guard compressingPath == nil else { return }
        compressingPath = item.path
        compressionProgress = 0
        lastCompressionSavings = nil
        compressionEstimateText = nil
        statusMessage = "Compressing \(item.displayName)…"
        let quality = videoQualityPreset
        let path = item.path
        let originalBytes = item.byteSize

        compressionTask = Task {
            if let estimated = await service.videoCompressor.estimate(path: path, quality: quality) {
                let before = ByteCountFormatter.string(fromByteCount: originalBytes, countStyle: .file)
                let after = ByteCountFormatter.string(fromByteCount: estimated, countStyle: .file)
                await MainActor.run {
                    if self.compressingPath == path {
                        self.compressionEstimateText = "Estimated \(before) → ~\(after)"
                    }
                }
            }

            let result = await service.videoCompressor.compress(path: path, quality: quality) { fraction in
                Task { @MainActor in
                    if self.compressingPath == path {
                        self.compressionProgress = fraction
                    }
                }
            }
            await MainActor.run {
                guard self.compressingPath == path else { return }
                self.compressingPath = nil
                self.compressionProgress = 0
                self.compressionEstimateText = nil
                self.compressionTask = nil
                if result.wasCancelled {
                    self.statusMessage = "Compression cancelled"
                    return
                }
                if result.success {
                    let before = ByteCountFormatter.string(fromByteCount: result.originalBytes, countStyle: .file)
                    let after = ByteCountFormatter.string(fromByteCount: result.compressedBytes, countStyle: .file)
                    let saved = ByteCountFormatter.string(fromByteCount: result.savedBytes, countStyle: .file)
                    self.lastCompressionSavings = "\(before) → \(after) (−\(result.savedPercent)%, saved \(saved))"
                    self.showToast("Compressed — \(before) → \(after) (−\(result.savedPercent)%). Original in Trash.")
                    self.scanLargeFiles()
                    self.refreshStorageOverview()
                    self.statusMessage = "Compression complete"
                } else {
                    self.errorMessage = result.errorMessage ?? "Compression failed"
                    self.statusMessage = "Compression failed"
                }
            }
        }
    }

    func compressImage(_ item: LargeFileItem) {
        guard item.isImage else { return }
        guard compressingPath == nil else { return }
        compressingPath = item.path
        compressionProgress = 0
        lastCompressionSavings = nil
        compressionEstimateText = nil
        statusMessage = "Compressing \(item.displayName)…"
        let path = item.path
        let format = imageOutputFormat
        let originalBytes = item.byteSize

        compressionTask = Task.detached(priority: .userInitiated) {
            let estimated = UninstallService.shared.imageCompressor.estimate(path: path, format: format)
            if let estimated {
                let before = ByteCountFormatter.string(fromByteCount: originalBytes, countStyle: .file)
                let after = ByteCountFormatter.string(fromByteCount: estimated, countStyle: .file)
                await MainActor.run {
                    if self.compressingPath == path {
                        self.compressionEstimateText = "Estimated \(before) → ~\(after)"
                    }
                }
            }

            let result = UninstallService.shared.imageCompressor.compress(path: path, format: format)
            await MainActor.run {
                guard self.compressingPath == path else { return }
                self.compressingPath = nil
                self.compressionProgress = 0
                self.compressionEstimateText = nil
                self.compressionTask = nil
                if result.wasCancelled {
                    self.statusMessage = "Compression cancelled"
                    return
                }
                if result.success {
                    let before = ByteCountFormatter.string(fromByteCount: result.originalBytes, countStyle: .file)
                    let after = ByteCountFormatter.string(fromByteCount: result.compressedBytes, countStyle: .file)
                    self.lastCompressionSavings = "\(before) → \(after) (−\(result.savedPercent)%)"
                    self.showToast("Image compressed — \(before) → \(after). Original in Trash.")
                    self.scanLargeFiles()
                    self.refreshStorageOverview()
                    self.statusMessage = "Compression complete"
                } else {
                    self.errorMessage = result.errorMessage ?? "Image compression failed"
                    self.statusMessage = "Compression failed"
                }
            }
        }
    }

    func cancelCompression() {
        service.videoCompressor.cancelActiveExport()
        service.imageCompressor.cancelActive()
        compressionTask?.cancel()
        compressionTask = nil
        if compressingPath != nil {
            compressingPath = nil
            compressionProgress = 0
            compressionEstimateText = nil
            statusMessage = "Compression cancelled"
        }
    }

    func refreshCLICleanupActions() {
        cliCleanupActions = service.cliCleanup.availableActions()
    }

    func requestCLICleanup(_ action: CLICleanupAction) {
        guard action.isAvailable else { return }
        pendingCLIAction = action
        showCLICleanupConfirm = true
    }

    func performCLICleanup() {
        guard let action = pendingCLIAction else { return }
        showCLICleanupConfirm = false
        isRunningCLICleanup = true
        statusMessage = "Running \(action.title)…"
        let toolID = action.toolID
        Task.detached(priority: .userInitiated) {
            let result = UninstallService.shared.cliCleanup.run(toolID)
            await MainActor.run {
                self.isRunningCLICleanup = false
                self.pendingCLIAction = nil
                self.cliCleanupSummary = result.summary
                if result.success {
                    self.showToast("\(action.title) finished")
                    self.statusMessage = "CLI cleanup complete"
                } else {
                    self.errorMessage = result.summary
                    self.statusMessage = "CLI cleanup failed"
                }
                self.scanDevDependencies()
                self.refreshStorageOverview()
                self.refreshCLICleanupActions()
            }
        }
    }

    // MARK: - Caches

    func scanCaches() {
        isScanningCaches = true
        statusMessage = "Scanning caches…"
        Task.detached(priority: .userInitiated) {
            let found = UninstallService.shared.scanCaches()
            await MainActor.run {
                self.cacheItems = found
                self.hasScannedCaches = true
                self.isScanningCaches = false
                self.statusMessage = "\(found.count) cache folders"
            }
        }
    }

    func toggleCache(_ item: CacheItem, selected: Bool) {
        guard let idx = cacheItems.firstIndex(where: { $0.id == item.id }) else { return }
        cacheItems[idx].isSelected = selected
    }

    func selectAllCaches(_ selected: Bool) {
        for i in cacheItems.indices {
            cacheItems[i].isSelected = selected
        }
    }

    func requestCacheRemoval() {
        guard !selectedCaches.isEmpty else { return }
        confirmMode = .caches
        showConfirmSheet = true
    }

    func removeSelectedCaches() {
        let selected = selectedCaches
        guard !selected.isEmpty else { return }
        isRemoving = true
        let paths = selected.map { (path: $0.path, category: ResourceCategory.userCache, byteSize: $0.byteSize) }
        Task {
            let report = await service.removal.trashPaths(paths, reportName: "Caches")
            await MainActor.run {
                self.lastReport = report
                self.selectedReportID = report.id
                self.isRemoving = false
                self.refreshReports()
                self.showToast("Moved \(report.removedCount) cache folder(s) to Trash")
                self.scanCaches()
                self.refreshStorageOverview()
            }
        }
    }

    // MARK: - Developer

    func scanDevDependencies() {
        isScanningDev = true
        statusMessage = "Scanning developer folders…"
        Task.detached(priority: .userInitiated) {
            let found = UninstallService.shared.scanDevDependencies()
            await MainActor.run {
                self.devItems = found
                self.hasScannedDev = true
                self.isScanningDev = false
                self.refreshCLICleanupActions()
                self.statusMessage = "\(found.count) developer items"
            }
        }
    }

    func toggleDevItem(_ item: DevDependencyItem, selected: Bool) {
        guard let idx = devItems.firstIndex(where: { $0.id == item.id }) else { return }
        devItems[idx].isSelected = selected
    }

    func requestDevRemoval() {
        guard !selectedDevItems.isEmpty else { return }
        confirmMode = .developer
        showConfirmSheet = true
    }

    func removeSelectedDevItems() {
        let selected = selectedDevItems
        guard !selected.isEmpty else { return }
        isRemoving = true
        let paths = selected.map { (path: $0.path, category: ResourceCategory.devDependency, byteSize: $0.byteSize) }
        Task {
            let report = await service.removal.trashPaths(paths, reportName: "Developer Dependencies")
            await MainActor.run {
                self.lastReport = report
                self.selectedReportID = report.id
                self.isRemoving = false
                self.refreshReports()
                self.showToast("Moved \(report.removedCount) developer item(s) to Trash")
                self.scanDevDependencies()
                self.refreshStorageOverview()
            }
        }
    }

    // MARK: - Apps / orphans (existing)

    func refreshApps() {
        isListingApps = true
        statusMessage = "Scanning Applications…"
        Task.detached(priority: .userInitiated) {
            let list = UninstallService.shared.listApplications()
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
        case .largeFiles:
            removeSelectedLargeFiles()
        case .caches:
            removeSelectedCaches()
        case .developer:
            removeSelectedDevItems()
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
                    self.refreshApps()
                    self.refreshStorageOverview()
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
                self.refreshStorageOverview()
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

    func openAutomationPrivacySettings() {
        if let url = URL(string: "x-apple.systempreferences:com.apple.preference.security?Privacy_Automation") {
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
