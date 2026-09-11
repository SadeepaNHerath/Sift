import SwiftUI
import UninstallerCore

struct ApplicationListView: View {
    @EnvironmentObject private var model: AppViewModel
    @State private var filter = ""

    var filtered: [AppIdentity] {
        if filter.isEmpty { return model.apps }
        return model.apps.filter {
            $0.name.localizedCaseInsensitiveContains(filter)
                || $0.bundleIdentifier.localizedCaseInsensitiveContains(filter)
        }
    }

    var body: some View {
        VStack(spacing: 0) {
            HStack {
                TextField("Filter apps", text: $filter)
                    .textFieldStyle(.roundedBorder)
                Button {
                    model.refreshApps()
                } label: {
                    Image(systemName: "arrow.clockwise")
                }
                .help("Rescan Applications")
            }
            .padding(8)

            List(filtered, selection: $model.selectedAppID) { app in
                VStack(alignment: .leading, spacing: 2) {
                    Text(app.name).font(.body.weight(.medium))
                    Text(app.bundleIdentifier)
                        .font(.caption)
                        .foregroundStyle(.secondary)
                        .lineLimit(1)
                }
                .tag(app.id)
            }
            .onChange(of: model.selectedAppID) { _, _ in
                model.analyzeSelected()
            }
        }
        .navigationTitle("Applications")
    }
}

struct AppDetailView: View {
    @EnvironmentObject private var model: AppViewModel

    var body: some View {
        Group {
            if let app = model.selectedApp {
                VStack(alignment: .leading, spacing: 0) {
                    header(app)
                    Divider()
                    if model.analysis == nil && model.isScanning {
                        ProgressView("Analyzing related resources…")
                            .frame(maxWidth: .infinity, maxHeight: .infinity)
                    } else if let analysis = model.analysis {
                        ResourceReviewView(resources: model.displayedResources)
                        Divider()
                        footer(analysis)
                    } else {
                        ContentUnavailableView(
                            "Select an application",
                            systemImage: "app.dashed",
                            description: Text("Choose an app or drop a .app onto the window to discover related files with confidence scoring.")
                        )
                    }
                }
            } else {
                ContentUnavailableView(
                    "No Applications",
                    systemImage: "tray",
                    description: Text("No third-party applications were found. Drop a .app here to analyze it.")
                )
            }
        }
    }

    private func header(_ app: AppIdentity) -> some View {
        HStack(alignment: .top) {
            VStack(alignment: .leading, spacing: 4) {
                Text(app.name).font(.title2.bold())
                Text(app.bundleIdentifier).font(.caption).foregroundStyle(.secondary)
                Text(app.path).font(.caption2).foregroundStyle(.tertiary).lineLimit(1)
                if let team = app.teamID {
                    Text("Team ID: \(team)").font(.caption2).foregroundStyle(.secondary)
                }
            }
            Spacer()
            Button("Re-analyze") { model.analyzeSelected() }
                .disabled(model.isScanning)
            Button("Move to Trash…") { model.requestUninstall() }
                .buttonStyle(.borderedProminent)
                .disabled(model.selectedCount == 0 || model.isRemoving)
        }
        .padding()
    }

    private func footer(_ analysis: AttributionResult) -> some View {
        HStack {
            Text("\(model.selectedCount) selected · \(ByteCountFormatter.string(fromByteCount: model.selectedBytes, countStyle: .file))")
            Spacer()
            Text("High \(analysis.resources(with: .high).count) · Medium \(analysis.resources(with: .medium).count) · Low \(analysis.resources(with: .low).count)")
                .font(.caption)
                .foregroundStyle(.secondary)
            Button("Dry Run") { model.performUninstall(dryRun: true) }
                .disabled(model.selectedCount == 0)
        }
        .padding()
    }
}

struct ResourceReviewView: View {
    @EnvironmentObject private var model: AppViewModel
    let resources: [AttributedResource]

    var body: some View {
        List {
            section(title: "High Confidence", level: .high, tint: .green)
            section(title: "Medium Confidence — review before deleting", level: .medium, tint: .orange)
            section(title: "Low Confidence — possibly shared", level: .low, tint: .red)
            deniedSection
        }
        .listStyle(.inset)
    }

    @ViewBuilder
    private func section(title: String, level: ConfidenceLevel, tint: Color) -> some View {
        let items = resources.filter { $0.confidence == level && !$0.isDenied }
        if !items.isEmpty {
            Section(title) {
                ForEach(items) { item in
                    ResourceRow(item: item, tint: tint)
                }
            }
        }
    }

    @ViewBuilder
    private var deniedSection: some View {
        let denied = resources.filter(\.isDenied)
        if !denied.isEmpty {
            Section("Protected / Denied") {
                ForEach(denied) { item in
                    VStack(alignment: .leading, spacing: 4) {
                        Text(item.resource.displayName).foregroundStyle(.secondary)
                        Text(item.denyReason ?? "Denied")
                            .font(.caption)
                            .foregroundStyle(.red)
                        Text(item.resource.path)
                            .font(.caption2)
                            .foregroundStyle(.tertiary)
                    }
                    .padding(.vertical, 2)
                }
            }
        }
    }
}

struct ResourceRow: View {
    @EnvironmentObject private var model: AppViewModel
    let item: AttributedResource
    let tint: Color

    var body: some View {
        HStack(alignment: .top, spacing: 10) {
            Toggle("", isOn: Binding(
                get: { item.isSelected },
                set: { model.setSelected(item, selected: $0) }
            ))
            .labelsHidden()
            .disabled(item.isDenied)

            VStack(alignment: .leading, spacing: 4) {
                HStack {
                    Text(item.resource.displayName).font(.body.weight(.medium))
                    if item.isPossiblyShared {
                        Text("possibly shared")
                            .font(.caption2)
                            .padding(.horizontal, 6)
                            .padding(.vertical, 2)
                            .background(Color.orange.opacity(0.2))
                            .clipShape(Capsule())
                    }
                    if item.resource.requiresPrivilege {
                        Text("requires helper")
                            .font(.caption2)
                            .padding(.horizontal, 6)
                            .padding(.vertical, 2)
                            .background(Color.purple.opacity(0.2))
                            .clipShape(Capsule())
                    }
                    Spacer()
                    Text(item.resource.formattedSize)
                        .font(.caption)
                        .foregroundStyle(.secondary)
                }
                Text(item.resource.category.displayName)
                    .font(.caption)
                    .foregroundStyle(tint)
                Text(item.resource.path)
                    .font(.caption2)
                    .foregroundStyle(.tertiary)
                    .lineLimit(2)
                EvidenceBadges(signals: item.signals)
            }
        }
        .padding(.vertical, 2)
    }
}

struct EvidenceBadges: View {
    let signals: [AttributionSignal]

    var body: some View {
        FlowLikeBadges(signals: signals)
    }
}

struct FlowLikeBadges: View {
    let signals: [AttributionSignal]

    var body: some View {
        FlexibleBadgeWrap(signals: signals)
    }
}

struct FlexibleBadgeWrap: View {
    let signals: [AttributionSignal]

    var body: some View {
        // Simple wrapping via LazyVGrid
        LazyVGrid(columns: [GridItem(.adaptive(minimum: 120), spacing: 4)], alignment: .leading, spacing: 4) {
            ForEach(signals, id: \.self) { signal in
                Text(signal.displayName)
                    .font(.caption2)
                    .padding(.horizontal, 6)
                    .padding(.vertical, 2)
                    .background(Color.accentColor.opacity(0.15))
                    .clipShape(RoundedRectangle(cornerRadius: 4))
                    .help("Evidence: \(signal.displayName) (weight \(String(format: "%.2f", signal.weight)))")
            }
        }
    }
}

struct ConfirmRemovalSheet: View {
    @EnvironmentObject private var model: AppViewModel

    var body: some View {
        VStack(alignment: .leading, spacing: 16) {
            Text("Confirm Uninstall").font(.title2.bold())
            Text("\(model.selectedCount) item(s) will be moved to Trash (\(ByteCountFormatter.string(fromByteCount: model.selectedBytes, countStyle: .file))).")
            Text("Medium and low confidence items you selected may affect other software. Review the evidence badges before continuing.")
                .foregroundStyle(.secondary)
            HStack {
                Button("Cancel", role: .cancel) { model.showConfirmSheet = false }
                Spacer()
                Button("Move to Trash", role: .destructive) {
                    model.performUninstall(dryRun: false)
                }
                .keyboardShortcut(.defaultAction)
            }
        }
        .padding(24)
        .frame(width: 480)
    }
}
