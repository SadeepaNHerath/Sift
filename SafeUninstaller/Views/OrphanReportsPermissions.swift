import SwiftUI
import UninstallerCore

struct OrphanListView: View {
    @EnvironmentObject private var model: AppViewModel

    var body: some View {
        VStack(spacing: 0) {
            HStack {
                Text("Leftovers from removed apps")
                    .font(.caption)
                    .foregroundStyle(.secondary)
                Spacer()
                Button("Scan") { model.scanOrphans() }
                    .disabled(model.isScanning)
            }
            .padding(8)

            List(model.orphans) { orphan in
                HStack(alignment: .top) {
                    Toggle("", isOn: Binding(
                        get: { orphan.isSelected },
                        set: { model.toggleOrphan(orphan, selected: $0) }
                    ))
                    .labelsHidden()
                    VStack(alignment: .leading, spacing: 2) {
                        Text(orphan.resource.displayName).font(.body.weight(.medium))
                        if let id = orphan.inferredBundleID {
                            Text(id).font(.caption).foregroundStyle(.secondary)
                        }
                        Text(orphan.confidence.displayName)
                            .font(.caption2)
                            .foregroundStyle(orphan.confidence == .high ? .green : (orphan.confidence == .medium ? .orange : .red))
                    }
                }
            }
        }
        .navigationTitle("Orphaned Files")
        .onAppear {
            if model.orphans.isEmpty {
                model.scanOrphans()
            }
        }
    }
}

struct OrphanDetailView: View {
    @EnvironmentObject private var model: AppViewModel

    var selected: [OrphanedResource] {
        model.orphans.filter(\.isSelected)
    }

    var body: some View {
        VStack(alignment: .leading, spacing: 12) {
            Text("Orphaned File Search").font(.title2.bold())
            Text("These entries look like application leftovers (bundle-ID shaped paths) that do not match any currently installed app. Nothing is deleted until you select items and confirm.")
                .foregroundStyle(.secondary)

            if model.orphans.isEmpty {
                ContentUnavailableView(
                    "No Orphans Found",
                    systemImage: "checkmark.seal",
                    description: Text("Run a scan to look for leftover preferences, containers, and support files.")
                )
            } else {
                List(model.orphans) { orphan in
                    VStack(alignment: .leading, spacing: 4) {
                        Text(orphan.resource.path).font(.callout)
                        HStack {
                            Text(orphan.resource.category.displayName)
                            Text(orphan.resource.formattedSize)
                            Text(orphan.confidence.displayName)
                        }
                        .font(.caption)
                        .foregroundStyle(.secondary)
                    }
                }
            }

            Spacer()
            HStack {
                Text("\(selected.count) selected")
                Spacer()
                Button("Move Selected to Trash", role: .destructive) {
                    model.removeSelectedOrphans()
                }
                .disabled(selected.isEmpty || model.isRemoving)
            }
        }
        .padding()
    }
}

struct ReportsListView: View {
    @EnvironmentObject private var model: AppViewModel
    @State private var selectedID: UUID?

    var body: some View {
        List(model.reports, selection: $selectedID) { report in
            VStack(alignment: .leading, spacing: 2) {
                Text(report.appName).font(.body.weight(.medium))
                Text(report.createdAt.formatted(date: .abbreviated, time: .shortened))
                    .font(.caption)
                    .foregroundStyle(.secondary)
                Text("Removed \(report.removedCount) · Failed \(report.failedCount)")
                    .font(.caption2)
            }
            .tag(report.id)
        }
        .navigationTitle("Reports")
        .onAppear { model.refreshReports() }
        .onChange(of: selectedID) { _, newValue in
            if let id = newValue {
                model.lastReport = model.reports.first { $0.id == id }
            }
        }
        .onChange(of: model.lastReport?.id) { _, newValue in
            selectedID = newValue
        }
    }
}

struct ReportDetailView: View {
    @EnvironmentObject private var model: AppViewModel

    var body: some View {
        if let report = model.lastReport {
            ScrollView {
                VStack(alignment: .leading, spacing: 12) {
                    Text("Audit Report").font(.title2.bold())
                    Text("\(report.appName) (\(report.bundleIdentifier))")
                    Text(report.createdAt.formatted())
                        .foregroundStyle(.secondary)
                    HStack {
                        metric("Removed", "\(report.removedCount)", .green)
                        metric("Retained", "\(report.retainedCount)", .secondary)
                        metric("Failed", "\(report.failedCount)", .red)
                        metric("Freed", ByteCountFormatter.string(fromByteCount: report.removedBytes, countStyle: .file), .blue)
                    }
                    if !report.notes.isEmpty {
                        Text(report.notes).font(.callout).foregroundStyle(.secondary)
                    }
                    Divider()
                    ForEach(report.items) { item in
                        VStack(alignment: .leading, spacing: 2) {
                            HStack {
                                Text(item.outcome.rawValue.uppercased())
                                    .font(.caption2.weight(.bold))
                                    .foregroundStyle(color(for: item.outcome))
                                Text(item.path).font(.caption).lineLimit(2)
                            }
                            if let err = item.errorMessage {
                                Text(err).font(.caption2).foregroundStyle(.red)
                            }
                        }
                        .padding(.vertical, 2)
                    }
                }
                .padding()
            }
        } else {
            ContentUnavailableView(
                "No Report Selected",
                systemImage: "doc.text",
                description: Text("Completed uninstalls appear here with evidence and outcomes.")
            )
        }
    }

    private func metric(_ title: String, _ value: String, _ color: Color) -> some View {
        VStack {
            Text(value).font(.headline).foregroundStyle(color)
            Text(title).font(.caption2).foregroundStyle(.secondary)
        }
        .frame(maxWidth: .infinity)
        .padding(8)
        .background(Color.secondary.opacity(0.08))
        .clipShape(RoundedRectangle(cornerRadius: 8))
    }

    private func color(for outcome: RemovalOutcome) -> Color {
        switch outcome {
        case .removed: return .green
        case .failed: return .red
        case .requiresHelper: return .purple
        case .skippedDenied: return .orange
        case .retained: return .secondary
        }
    }
}

struct PermissionsView: View {
    var body: some View {
        List {
            Text("Grant Full Disk Access and optionally register the privileged helper for /Library cleanup.")
                .font(.caption)
                .foregroundStyle(.secondary)
        }
        .navigationTitle("Permissions")
    }
}

struct PermissionsDetailView: View {
    @EnvironmentObject private var model: AppViewModel

    var body: some View {
        Form {
            Section("Full Disk Access") {
                LabeledContent("Status") {
                    Text(model.hasFDA ? "Likely granted" : "Not detected")
                        .foregroundStyle(model.hasFDA ? .green : .orange)
                }
                Button("Open Full Disk Access Settings") { model.openFullDiskAccessSettings() }
                Button("Recheck") { model.refreshPermissions() }
            }
            Section("Privileged Helper") {
                LabeledContent("Status") {
                    Text(model.helperStatus)
                }
                Text("Needed only for root-owned files under /Library. User-domain cleanup works without it. Ad-hoc signed builds may show Not Found until you sign with an Apple Development certificate.")
                    .font(.caption)
                    .foregroundStyle(.secondary)
                Button("Register Helper") { model.registerHelper() }
                Button("Open Login Items Settings") { model.openLoginItemsSettings() }
            }
            Section("Safety") {
                Text("All deletions move items to Trash. Apple/system paths and shared Library roots are denylisted. Medium/Low confidence items are unchecked by default.")
                    .font(.caption)
                    .foregroundStyle(.secondary)
            }
        }
        .formStyle(.grouped)
        .padding()
    }
}
