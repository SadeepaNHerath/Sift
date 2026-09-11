import SwiftUI
import AppKit
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
            List(filtered, selection: $model.selectedAppID) { app in
                HStack(spacing: 10) {
                    Image(nsImage: model.icon(for: app))
                        .resizable()
                        .frame(width: 28, height: 28)
                    VStack(alignment: .leading, spacing: 2) {
                        Text(app.name).font(.body.weight(.medium))
                        Text(app.version.isEmpty ? app.bundleIdentifier : "\(app.version) · \(app.bundleIdentifier)")
                            .font(.caption)
                            .foregroundStyle(.secondary)
                            .lineLimit(1)
                    }
                }
                .tag(app.id)
            }
        }
        .searchable(text: $filter, prompt: "Filter apps")
        .navigationTitle("Applications")
        .toolbar {
            ToolbarItem {
                Button {
                    model.refreshApps()
                } label: {
                    Image(systemName: "arrow.clockwise")
                }
                .help("Rescan Applications")
            }
        }
        .onChange(of: model.selectedAppID) { _, _ in
            model.analyzeSelected()
        }
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
                    if model.isAnalyzing && model.analysis == nil {
                        ProgressView("Analyzing related resources…")
                            .frame(maxWidth: .infinity, maxHeight: .infinity)
                    } else if model.analysis != nil {
                        ResourceReviewView(resources: model.displayedResources)
                        Divider()
                        footer
                    } else {
                        dropHint
                    }
                }
            } else {
                ContentUnavailableView(
                    "No Applications",
                    systemImage: "tray",
                    description: Text("No third-party applications were found. Drop a .app onto the window to analyze it.")
                )
            }
        }
    }

    private var dropHint: some View {
        ContentUnavailableView(
            "Select an application",
            systemImage: "app.dashed",
            description: Text("Choose an app from the list, or drop a .app onto this window to discover related files with confidence scoring.")
        )
    }

    private func header(_ app: AppIdentity) -> some View {
        HStack(alignment: .top, spacing: 14) {
            Image(nsImage: model.icon(for: app))
                .resizable()
                .frame(width: 48, height: 48)
            VStack(alignment: .leading, spacing: 4) {
                Text(app.name).font(.title2.bold())
                Text(app.bundleIdentifier).font(.caption).foregroundStyle(.secondary)
                Text(app.path).font(.caption2).foregroundStyle(.tertiary).lineLimit(1)
                HStack(spacing: 8) {
                    if !app.version.isEmpty {
                        Text("v\(app.version)").font(.caption2).foregroundStyle(.secondary)
                    }
                    if let team = app.teamID {
                        Text("Team \(team)").font(.caption2).foregroundStyle(.secondary)
                    }
                    permissionChip(model.hasFDA ? "Full Disk Access" : "Limited access", ok: model.hasFDA)
                }
            }
            Spacer()
            Button("Re-analyze") { model.analyzeSelected() }
                .disabled(model.isAnalyzing)
            Button("Review & Move to Trash…") { model.requestUninstall() }
                .buttonStyle(.borderedProminent)
                .disabled(model.selectedCount == 0 || model.isRemoving)
        }
        .padding()
    }

    private func permissionChip(_ title: String, ok: Bool) -> some View {
        Text(title)
            .font(.caption2)
            .padding(.horizontal, 6)
            .padding(.vertical, 2)
            .background(ok ? Color.green.opacity(0.15) : Color.orange.opacity(0.15))
            .clipShape(RoundedRectangle(cornerRadius: 4))
    }

    private var footer: some View {
        HStack {
            Text("\(model.selectedCount) selected · \(ByteCountFormatter.string(fromByteCount: model.selectedBytes, countStyle: .file))")
            Spacer()
            if let analysis = model.analysis {
                Text("High \(analysis.resources(with: .high).count) · Medium \(analysis.resources(with: .medium).count) · Low \(analysis.resources(with: .low).count)")
                    .font(.caption)
                    .foregroundStyle(.secondary)
            }
            Button("Dry Run") {
                model.confirmMode = .uninstall
                model.performUninstall(dryRun: true)
            }
            .disabled(model.selectedCount == 0)
            .help("Simulate removal without moving files")
        }
        .padding()
    }
}

struct ResourceReviewView: View {
    @EnvironmentObject private var model: AppViewModel
    let resources: [AttributedResource]

    var body: some View {
        List {
            section(title: "High Confidence", level: .high, tint: .green, collapsed: false)
            section(title: "Medium Confidence — review before removing", level: .medium, tint: .orange, collapsed: false)
            DisclosureGroup(isExpanded: $model.lowConfidenceExpanded) {
                ForEach(resources.filter { $0.confidence == .low && !$0.isDenied }) { item in
                    ResourceRow(item: item, tint: .red)
                }
            } label: {
                sectionHeader(title: "Low Confidence — possibly shared", level: .low)
            }
            deniedSection
        }
        .listStyle(.inset)
    }

    @ViewBuilder
    private func section(title: String, level: ConfidenceLevel, tint: Color, collapsed: Bool) -> some View {
        let items = resources.filter { $0.confidence == level && !$0.isDenied }
        if !items.isEmpty {
            Section {
                ForEach(items) { item in
                    ResourceRow(item: item, tint: tint)
                }
            } header: {
                sectionHeader(title: title, level: level)
            }
        }
    }

    private func sectionHeader(title: String, level: ConfidenceLevel) -> some View {
        HStack {
            Text(title)
            Spacer()
            Button("Select") { model.selectAll(confidence: level, selected: true) }
                .buttonStyle(.borderless)
                .font(.caption)
            Button("Deselect") { model.selectAll(confidence: level, selected: false) }
                .buttonStyle(.borderless)
                .font(.caption)
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
            Toggle(isOn: Binding(
                get: { item.isSelected },
                set: { model.setSelected(item, selected: $0) }
            )) {
                EmptyView()
            }
            .labelsHidden()
            .disabled(item.isDenied)
            .accessibilityLabel(item.resource.displayName)

            VStack(alignment: .leading, spacing: 4) {
                HStack {
                    Text(item.resource.displayName).font(.body.weight(.medium))
                    if item.isPossiblyShared {
                        badge("possibly shared", color: .orange)
                    }
                    if item.resource.requiresPrivilege {
                        badge("requires helper", color: .purple)
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
                    .contextMenu {
                        Button("Reveal in Finder") { model.revealInFinder(item.resource.path) }
                    }
                FlexibleBadgeWrap(signals: item.signals)
            }
        }
        .padding(.vertical, 2)
    }

    private func badge(_ text: String, color: Color) -> some View {
        Text(text)
            .font(.caption2)
            .padding(.horizontal, 6)
            .padding(.vertical, 2)
            .background(color.opacity(0.2))
            .clipShape(RoundedRectangle(cornerRadius: 4))
    }
}

struct FlexibleBadgeWrap: View {
    let signals: [AttributionSignal]

    var body: some View {
        LazyVGrid(columns: [GridItem(.adaptive(minimum: 110), spacing: 4)], alignment: .leading, spacing: 4) {
            ForEach(signals, id: \.self) { signal in
                Text(signal.displayName)
                    .font(.caption2)
                    .padding(.horizontal, 6)
                    .padding(.vertical, 2)
                    .background(Color.accentColor.opacity(0.12))
                    .clipShape(RoundedRectangle(cornerRadius: 4))
                    .help(signal.displayName)
            }
        }
    }
}

struct ConfirmRemovalSheet: View {
    @EnvironmentObject private var model: AppViewModel

    private var count: Int {
        model.confirmMode == .uninstall ? model.selectedCount : model.selectedOrphans.count
    }

    private var bytes: Int64 {
        if model.confirmMode == .uninstall {
            return model.selectedBytes
        }
        return model.selectedOrphans.reduce(0) { $0 + $1.resource.byteSize }
    }

    var body: some View {
        VStack(alignment: .leading, spacing: 16) {
            Text(model.confirmMode == .uninstall ? "Move to Trash?" : "Remove leftovers?")
                .font(.title2.bold())
            Text("\(count) item(s) · \(ByteCountFormatter.string(fromByteCount: bytes, countStyle: .file)) will be moved to Trash. You can restore them from Trash later.")
                .foregroundStyle(.secondary)

            if model.confirmMode == .uninstall {
                let high = model.displayedResources.filter { $0.isSelected && $0.confidence == .high }.count
                let med = model.displayedResources.filter { $0.isSelected && $0.confidence == .medium }.count
                let low = model.displayedResources.filter { $0.isSelected && $0.confidence == .low }.count
                Text("Selected: \(high) high · \(med) medium · \(low) low")
                    .font(.caption)
            }

            GroupBox("Preview") {
                VStack(alignment: .leading, spacing: 4) {
                    ForEach(Array(model.confirmPreviewItems.enumerated()), id: \.offset) { _, item in
                        HStack {
                            Text(item.confidence)
                                .font(.caption2)
                                .frame(width: 56, alignment: .leading)
                                .foregroundStyle(.secondary)
                            Text((item.path as NSString).lastPathComponent)
                                .font(.caption)
                                .lineLimit(1)
                        }
                    }
                    if count > model.confirmPreviewItems.count {
                        Text("…and \(count - model.confirmPreviewItems.count) more")
                            .font(.caption2)
                            .foregroundStyle(.secondary)
                    }
                }
                .frame(maxWidth: .infinity, alignment: .leading)
            }

            Text("Medium and low confidence items may belong to other software. Review carefully.")
                .font(.caption)
                .foregroundStyle(.secondary)

            HStack {
                Button("Cancel", role: .cancel) { model.showConfirmSheet = false }
                    .keyboardShortcut(.cancelAction)
                Spacer()
                Button("Move to Trash", role: .destructive) {
                    model.performConfirmedAction(dryRun: false)
                }
            }
        }
        .padding(24)
        .frame(width: 520)
    }
}
