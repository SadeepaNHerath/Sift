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
            .navigationSplitViewColumnWidth(min: 160, ideal: 180, max: 220)
        } content: {
            switch model.sidebar {
            case .applications:
                ApplicationListView()
            case .leftovers:
                OrphanListView()
            case .history:
                ReportsListView()
            }
        } detail: {
            switch model.sidebar {
            case .applications:
                AppDetailView()
            case .leftovers:
                OrphanDetailView()
            case .history:
                ReportDetailView()
            }
        }
        .toolbar {
            ToolbarItemGroup(placement: .primaryAction) {
                if model.sidebar == .applications {
                    Button("Setup") { model.showSetupSheet = true }
                }
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
            if let toast = model.toastMessage {
                ToastBanner(
                    message: toast,
                    onOpenTrash: { model.openTrash() },
                    onViewReport: {
                        model.sidebar = .history
                        model.toastMessage = nil
                    }
                )
                .padding()
                .transition(.move(edge: .bottom).combined(with: .opacity))
            }
        }
        .animation(.easeInOut, value: model.toastMessage)
        .alert("Something went wrong", isPresented: Binding(
            get: { model.errorMessage != nil },
            set: { if !$0 { model.errorMessage = nil } }
        )) {
            Button("OK", role: .cancel) { model.errorMessage = nil }
        } message: {
            Text(model.errorMessage ?? "")
        }
        .sheet(isPresented: $model.showConfirmSheet) {
            ConfirmRemovalSheet()
                .environmentObject(model)
        }
        .sheet(isPresented: $model.showSetupSheet) {
            SetupSheet()
                .environmentObject(model)
        }
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
        .shadow(radius: 8, y: 2)
        .frame(maxWidth: 520)
    }
}

struct SetupSheet: View {
    @EnvironmentObject private var model: AppViewModel

    var body: some View {
        VStack(alignment: .leading, spacing: 20) {
            Text("Welcome to SafeUninstaller")
                .font(.title2.bold())
            Text("Ad-hoc / unsigned builds are fully supported for cleaning your personal Library. Nothing is deleted until you review and confirm.")
                .foregroundStyle(.secondary)

            GroupBox("Full Disk Access") {
                VStack(alignment: .leading, spacing: 8) {
                    HStack {
                        Image(systemName: model.hasFDA ? "checkmark.seal.fill" : "exclamationmark.triangle.fill")
                            .foregroundStyle(model.hasFDA ? .green : .orange)
                        Text(model.hasFDA ? "Granted" : "Not granted")
                            .fontWeight(.medium)
                    }
                    Text("Needed to scan protected Library folders for related files.")
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
                        Text("Needs a paid Apple Developer ID certificate. Without it, SafeUninstaller still cleans user Library data normally.")
                            .font(.caption2)
                            .foregroundStyle(.orange)
                    }
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
        .frame(width: 480)
        .onAppear { model.refreshPermissions() }
    }
}
