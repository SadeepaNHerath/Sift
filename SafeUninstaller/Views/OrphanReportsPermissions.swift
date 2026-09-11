import SwiftUI
import UninstallerCore

struct OrphanListView: View {
    @EnvironmentObject private var model: AppViewModel
    @State private var filter = ""

    var filtered: [OrphanedResource] {
        if filter.isEmpty { return model.orphans }
        return model.orphans.filter {
            $0.resource.path.localizedCaseInsensitiveContains(filter)
                || ($0.inferredBundleID?.localizedCaseInsensitiveContains(filter) ?? false)
        }
    }

    var body: some View {
        VStack(spacing: 0) {
            List(filtered, selection: $model.selectedOrphanID) { orphan in
                HStack(alignment: .top) {
                    Toggle(isOn: Binding(
                        get: { orphan.isSelected },
                        set: { model.toggleOrphan(orphan, selected: $0) }
                    )) {
                        EmptyView()
                    }
                    .labelsHidden()
                    .accessibilityLabel(orphan.resource.displayName)

                    VStack(alignment: .leading, spacing: 2) {
                        Text(orphan.resource.displayName).font(.body.weight(.medium))
                        if let id = orphan.inferredBundleID {
                            Text(id).font(.caption).foregroundStyle(.secondary).lineLimit(1)
                        }
                        Text(orphan.confidence.displayName)
                            .font(.caption2)
                            .foregroundStyle(color(for: orphan.confidence))
                    }
                }
                .tag(orphan.id)
            }
        }
        .searchable(text: $filter, prompt: "Filter leftovers")
        .navigationTitle("Leftovers")
        .toolbar {
            ToolbarItem {
                Button("Scan") { model.scanOrphans() }
                    .disabled(model.isScanningOrphans)
            }
        }
        .onAppear {
            if !model.hasScannedOrphans {
                model.scanOrphans()
            }
        }
    }

    private func color(for level: ConfidenceLevel) -> Color {
        switch level {
        case .high: return .green
        case .medium: return .orange
        case .low: return .red
        }
    }
}

struct OrphanDetailView: View {
    @EnvironmentObject private var model: AppViewModel

    var body: some View {
        VStack(alignment: .leading, spacing: 0) {
            HStack {
                VStack(alignment: .leading, spacing: 4) {
                    Text("Leftovers").font(.title2.bold())
                    Text("Files that look like application data but don’t match any installed app. Review carefully — confidence is intentionally conservative.")
                        .font(.callout)
                        .foregroundStyle(.secondary)
                }
                Spacer()
                Button("Move Selected to Trash…", role: .destructive) {
                    model.requestOrphanRemoval()
                }
                .disabled(model.selectedOrphans.isEmpty || model.isRemoving)
            }
            .padding()

            Divider()

            if model.isScanningOrphans {
                ProgressView("Scanning…")
                    .frame(maxWidth: .infinity, maxHeight: .infinity)
            } else if !model.hasScannedOrphans {
                ContentUnavailableView(
                    "Not scanned yet",
                    systemImage: "magnifyingglass",
                    description: Text("Run a scan to look for leftover preferences, containers, and support files.")
                )
            } else if model.orphans.isEmpty {
                ContentUnavailableView(
                    "No leftovers found",
                    systemImage: "checkmark.seal",
                    description: Text("Nothing orphan-like turned up in the usual Library locations.")
                )
            } else if let orphan = model.selectedOrphan {
                Form {
                    Section("Selected leftover") {
                        LabeledContent("Name", value: orphan.resource.displayName)
                        LabeledContent("Category", value: orphan.resource.category.displayName)
                        LabeledContent("Size", value: orphan.resource.formattedSize)
                        LabeledContent("Confidence", value: orphan.confidence.displayName)
                        if let id = orphan.inferredBundleID {
                            LabeledContent("Inferred ID", value: id)
                        }
                        LabeledContent("Path") {
                            Text(orphan.resource.path)
                                .font(.caption)
                                .textSelection(.enabled)
                        }
                        Button("Reveal in Finder") {
                            model.revealInFinder(orphan.resource.path)
                        }
                    }
                    Section("Batch") {
                        Text("\(model.selectedOrphans.count) item(s) checked in the list will be moved when you confirm.")
                            .font(.caption)
                            .foregroundStyle(.secondary)
                    }
                }
                .formStyle(.grouped)
            } else {
                ContentUnavailableView("Select a leftover", systemImage: "doc")
            }
        }
    }
}

struct ReportsListView: View {
    @EnvironmentObject private var model: AppViewModel

    var body: some View {
        List(model.reports, selection: $model.selectedReportID) { report in
            VStack(alignment: .leading, spacing: 2) {
                HStack {
                    Text(report.appName).font(.body.weight(.medium))
                    if report.notes.lowercased().contains("dry") {
                        Text("DRY RUN")
                            .font(.caption2.weight(.bold))
                            .padding(.horizontal, 5)
                            .padding(.vertical, 1)
                            .background(Color.blue.opacity(0.15))
                            .clipShape(RoundedRectangle(cornerRadius: 3))
                    }
                }
                Text(report.createdAt.formatted(date: .abbreviated, time: .shortened))
                    .font(.caption)
                    .foregroundStyle(.secondary)
                Text("Removed \(report.removedCount) · Failed \(report.failedCount)")
                    .font(.caption2)
            }
            .tag(report.id)
        }
        .navigationTitle("History")
        .onAppear { model.refreshReports() }
        .onChange(of: model.selectedReportID) { _, newValue in
            if let id = newValue {
                model.lastReport = model.reports.first { $0.id == id }
            }
        }
    }
}

struct ReportDetailView: View {
    @EnvironmentObject private var model: AppViewModel

    var body: some View {
        if let report = model.lastReport ?? model.reports.first {
            ScrollView {
                VStack(alignment: .leading, spacing: 12) {
                    Text("Audit Report").font(.title2.bold())
                    Text("\(report.appName) (\(report.bundleIdentifier))")
                    Text(report.createdAt.formatted())
                        .foregroundStyle(.secondary)
                    if report.notes.lowercased().contains("dry") {
                        Text("Dry run — files were not moved.")
                            .font(.callout)
                            .foregroundStyle(.blue)
                    }
                    HStack {
                        metric("Removed", "\(report.removedCount)", .green)
                        metric("Retained", "\(report.retainedCount)", .secondary)
                        metric("Failed", "\(report.failedCount)", .red)
                        metric("Freed", ByteCountFormatter.string(fromByteCount: report.removedBytes, countStyle: .file), .blue)
                    }
                    Text("Reports store full paths on this Mac only (folder permissions 0700). Avoid sharing exports if paths are sensitive.")
                        .font(.caption2)
                        .foregroundStyle(.tertiary)
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
            .onAppear {
                if model.selectedReportID == nil {
                    model.selectedReportID = report.id
                    model.lastReport = report
                }
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
