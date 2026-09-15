import SwiftUI
import UninstallerCore

struct ContentView: View {
    @EnvironmentObject private var model: AppViewModel

    var body: some View {
        NavigationSplitView {
            List(selection: $model.sidebar) {
                ForEach(AppViewModel.SidebarItem.allCases) { item in
                    Label(item.rawValue, systemImage: item.systemImage)
                        .tag(item)
                        .accessibilityIdentifier("sidebar.\(item.rawValue)")
                }
                if !model.hasFDA {
                    Divider()
                    Button {
                        model.showSetupSheet = true
                    } label: {
                        Label("Setup", systemImage: "lock.shield")
                    }
                    .buttonStyle(.plain)
                }
            }
            .listStyle(.sidebar)
            .navigationSplitViewColumnWidth(min: 180, ideal: 200, max: 260)
        } content: {
            Group {
                switch model.sidebar {
                case .overview:
                    OverviewSidebarList()
                case .applications:
                    ApplicationListView()
                case .leftovers:
                    OrphanListView()
                case .largeFiles:
                    LargeFilesPane()
                case .caches:
                    CachesPane()
                case .developer:
                    DeveloperPane()
                case .history:
                    ReportsListView()
                }
            }
            .navigationSplitViewColumnWidth(ideal: 340)
        } detail: {
            switch model.sidebar {
            case .overview:
                OverviewView()
            case .applications:
                AppDetailView()
            case .leftovers:
                OrphanDetailView()
            case .largeFiles:
                LargeFilesDetailView()
            case .caches:
                CachesDetailView()
            case .developer:
                DeveloperDetailView()
            case .history:
                ReportDetailView()
            }
        }
        .navigationSplitViewStyle(.balanced)
        .toolbar {
            ToolbarItemGroup(placement: .primaryAction) {
                Button("Setup") { model.showSetupSheet = true }
            }
            ToolbarItem(placement: .status) {
                HStack(spacing: 8) {
                    if model.isBusy {
                        ProgressView().controlSize(.small)
                    }
                    Text(model.statusMessage)
                        .font(.caption)
                        .foregroundStyle(.secondary)
                        .lineLimit(1)
                }
            }
        }
        .overlay {
            if model.isDropTargeted {
                RoundedRectangle(cornerRadius: 12)
                    .strokeBorder(Color.accentColor, lineWidth: 3)
                    .background(Color.accentColor.opacity(0.08))
                    .padding(8)
                    .allowsHitTesting(false)
                VStack {
                    Image(systemName: "app.dashed")
                        .font(.largeTitle)
                    Text("Drop .app to analyze")
                        .font(.headline)
                }
                .allowsHitTesting(false)
            }
        }
        .overlay(alignment: .bottom) {
            VStack(spacing: 8) {
                if let banner = model.trashBannerMessage {
                    TrashBanner(
                        message: banner,
                        onOpenTrash: { model.openTrash() },
                        onViewReport: {
                            model.sidebar = .history
                            model.dismissTrashBanner()
                        },
                        onDismiss: { model.dismissTrashBanner() }
                    )
                    .padding(.horizontal)
                }
                if let toast = model.toastMessage {
                    ToastBanner(
                        message: toast,
                        onOpenTrash: { model.openTrash() },
                        onViewReport: {
                            model.sidebar = .history
                            model.toastMessage = nil
                        }
                    )
                    .padding(.horizontal)
                    .transition(.move(edge: .bottom).combined(with: .opacity))
                }
            }
            .padding(.bottom)
        }
        .animation(.easeInOut, value: model.toastMessage)
        .animation(.easeInOut, value: model.trashBannerMessage)
        .alert("Something went wrong", isPresented: Binding(
            get: { model.errorMessage != nil },
            set: { if !$0 { model.errorMessage = nil } }
        )) {
            Button("OK", role: .cancel) { model.errorMessage = nil }
        } message: {
            Text(model.errorMessage ?? "")
        }
        .alert("Updates unavailable", isPresented: $model.showUpdatesUnavailableAlert) {
            Button("OK", role: .cancel) {}
        } message: {
            Text("Sparkle auto-update is not configured for this build (empty SUFeedURL / SUPublicEDKey). Install a release packaged with update keys, or check GitHub Releases manually.")
        }
        .confirmationDialog(
            "Empty Trash?",
            isPresented: $model.showEmptyTrashConfirm,
            titleVisibility: .visible
        ) {
            Button("Empty Trash", role: .destructive) { model.emptyTrash() }
            Button("Cancel", role: .cancel) {}
        } message: {
            Text("This permanently deletes items in Trash and frees \(ByteCountFormatter.string(fromByteCount: model.trashBytes, countStyle: .file)). This cannot be undone.")
        }
        .sheet(isPresented: $model.showConfirmSheet) {
            ConfirmRemovalSheet()
                .environmentObject(model)
        }
        .sheet(isPresented: $model.showCLICleanupConfirm) {
            CLICleanupConfirmSheet()
                .environmentObject(model)
        }
        .sheet(isPresented: $model.showSetupSheet) {
            SetupSheet()
                .environmentObject(model)
        }
        .onChange(of: model.sidebar) { _, newValue in
            if newValue != .largeFiles {
                model.cancelCompression()
            }
        }
    }
}

struct OverviewSidebarList: View {
    @EnvironmentObject private var model: AppViewModel

    var body: some View {
        List {
            if let overview = model.storageOverview {
                Section("Disk") {
                    LabeledContent("Free", value: overview.formattedAvailable)
                    LabeledContent("Total", value: overview.formattedTotal)
                }
                Section("Categories") {
                    ForEach(overview.categories) { cat in
                        LabeledContent(cat.kind.displayName, value: cat.formattedSize)
                    }
                }
            } else {
                Text("Measuring storage…")
                    .foregroundStyle(.secondary)
            }
        }
        .navigationTitle("Storage")
    }
}

struct ToastBanner: View {
    let message: String
    var onOpenTrash: () -> Void
    var onViewReport: () -> Void

    var body: some View {
        HStack(spacing: 12) {
            Image(systemName: "checkmark.circle.fill")
                .foregroundStyle(.green)
            Text(message)
                .font(.callout)
            Spacer()
            Button("Open Trash", action: onOpenTrash)
            Button("View Report", action: onViewReport)
        }
        .padding(12)
        .background(.regularMaterial, in: RoundedRectangle(cornerRadius: 10))
        .frame(maxWidth: 520)
    }
}

struct TrashBanner: View {
    let message: String
    var onOpenTrash: () -> Void
    var onViewReport: () -> Void
    var onDismiss: () -> Void

    var body: some View {
        HStack(spacing: 12) {
            Image(systemName: "trash")
                .foregroundStyle(.orange)
            Text(message)
                .font(.callout)
            Spacer()
            Button("Open Trash", action: onOpenTrash)
            Button("View Report", action: onViewReport)
            Button("Dismiss", action: onDismiss)
        }
        .padding(12)
        .background(.regularMaterial, in: RoundedRectangle(cornerRadius: 10))
        .frame(maxWidth: 560)
    }
}

struct SetupSheet: View {
    @EnvironmentObject private var model: AppViewModel

    var body: some View {
        VStack(alignment: .leading, spacing: 20) {
            Text("Welcome to SafeUninstaller")
                .font(.title2.bold())
            Text("Free space by uninstalling apps completely, clearing caches, compressing videos, and removing developer leftovers you no longer need. Nothing is deleted until you review and confirm.")
                .foregroundStyle(.secondary)

            GroupBox("Unsigned build") {
                VStack(alignment: .leading, spacing: 8) {
                    Text("This build is ad-hoc signed (no paid Apple Developer certificate). The first time you open it, macOS Gatekeeper may block the app — use Right-click → Open → Open once.")
                        .font(.caption)
                        .foregroundStyle(.secondary)
                    Text("The /Library privileged helper stays unavailable without a Developer ID. Personal ~/Library cleanup, caches, large files, and developer tools still work.")
                        .font(.caption)
                        .foregroundStyle(.secondary)
                    Text("Diagnostics stay on this Mac (Help → Copy Diagnostics). No telemetry is sent.")
                        .font(.caption2)
                        .foregroundStyle(.tertiary)
                }
                .frame(maxWidth: .infinity, alignment: .leading)
                .padding(4)
            }

            GroupBox("Full Disk Access") {
                VStack(alignment: .leading, spacing: 8) {
                    HStack {
                        Image(systemName: model.hasFDA ? "checkmark.seal.fill" : "exclamationmark.triangle.fill")
                            .foregroundStyle(model.hasFDA ? .green : .orange)
                        Text(model.hasFDA ? "Granted" : "Not granted")
                            .fontWeight(.medium)
                    }
                    Text("Needed to scan protected Library folders accurately.")
                        .font(.caption)
                        .foregroundStyle(.secondary)
                    HStack {
                        Button("Open System Settings") { model.openFullDiskAccessSettings() }
                        Button("Recheck") { model.refreshPermissions() }
                    }
                }
                .frame(maxWidth: .infinity, alignment: .leading)
                .padding(4)
            }

            GroupBox("Privileged Helper (optional)") {
                VStack(alignment: .leading, spacing: 8) {
                    Text(model.helperStatus)
                        .fontWeight(.medium)
                    Text("Only needed for root-owned files under /Library. Personal ~/Library cleanup does not need this helper.")
                        .font(.caption)
                        .foregroundStyle(.secondary)
                    HStack {
                        Button("Register Helper") { model.registerHelper() }
                            .disabled(!model.canRegisterHelper)
                        Button("Login Items…") { model.openLoginItemsSettings() }
                    }
                    if !model.canRegisterHelper {
                        Text("Needs a paid Apple Developer ID certificate. Without it, user-Library cleanup still works.")
                            .font(.caption2)
                            .foregroundStyle(.orange)
                    }
                }
                .frame(maxWidth: .infinity, alignment: .leading)
                .padding(4)
            }

            GroupBox("Automation (Empty Trash)") {
                VStack(alignment: .leading, spacing: 8) {
                    Text("Empty Trash asks Finder to permanently delete items. macOS may prompt once for Automation permission so SafeUninstaller can control Finder.")
                        .font(.caption)
                        .foregroundStyle(.secondary)
                    Button("Open Privacy…") { model.openAutomationPrivacySettings() }
                }
                .frame(maxWidth: .infinity, alignment: .leading)
                .padding(4)
            }

            HStack {
                Button("Not Now") {
                    model.dismissSetup(permanently: false)
                    model.showSetupSheet = false
                }
                Spacer()
                Button("Start Cleaning") {
                    model.showSetupSheet = false
                    if model.hasFDA {
                        model.dismissSetup(permanently: true)
                    }
                }
                .keyboardShortcut(.defaultAction)
                .buttonStyle(.borderedProminent)
            }
        }
        .padding(24)
        .frame(width: 500)
        .onAppear { model.refreshPermissions() }
    }
}
