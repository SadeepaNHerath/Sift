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
            }
            .listStyle(.sidebar)
            .navigationSplitViewColumnWidth(min: 160, ideal: 180, max: 220)
        } content: {
            switch model.sidebar {
            case .applications:
                ApplicationListView()
            case .orphans:
                OrphanListView()
            case .reports:
                ReportsListView()
            case .settings:
                PermissionsView()
            }
        } detail: {
            switch model.sidebar {
            case .applications:
                AppDetailView()
            case .orphans:
                OrphanDetailView()
            case .reports:
                ReportDetailView()
            case .settings:
                PermissionsDetailView()
            }
        }
        .toolbar {
            ToolbarItem(placement: .status) {
                HStack(spacing: 8) {
                    if model.isScanning || model.isRemoving {
                        ProgressView()
                            .controlSize(.small)
                    }
                    Text(model.statusMessage)
                        .font(.caption)
                        .foregroundStyle(.secondary)
                        .lineLimit(1)
                }
            }
        }
        .alert("Error", isPresented: Binding(
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
        .overlay(alignment: .top) {
            if !model.hasFDA {
                FDABanner()
            }
        }
    }
}

struct FDABanner: View {
    @EnvironmentObject private var model: AppViewModel

    var body: some View {
        HStack {
            Image(systemName: "exclamationmark.shield")
            Text("Full Disk Access is recommended for complete Library scanning.")
                .font(.callout)
            Spacer()
            Button("Open Settings") { model.openFullDiskAccessSettings() }
            Button("Recheck") { model.refreshPermissions() }
        }
        .padding(10)
        .background(.yellow.opacity(0.85))
        .foregroundStyle(.black)
    }
}
