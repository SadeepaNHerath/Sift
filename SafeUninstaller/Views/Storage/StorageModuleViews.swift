import SwiftUI
import AppKit
import UninstallerCore

struct LargeFilesPane: View {
    @EnvironmentObject private var model: AppViewModel

    var body: some View {
        VStack(spacing: 0) {
            HStack {
                Text("Files over \(thresholdLabel) in common folders. Convert formats are in the detail pane.")
                    .font(.caption)
                    .foregroundStyle(.secondary)
                Spacer()
                Button("Select All") { model.selectAllLargeFiles(true) }
                Button("Scan") { model.scanLargeFiles() }
                    .disabled(model.isScanningLargeFiles)
            }
            .padding(8)

            if model.isScanningLargeFiles {
                ProgressView("Scanning…").frame(maxWidth: .infinity, maxHeight: .infinity)
            } else if !model.hasScannedLargeFiles {
                ContentUnavailableView("Not scanned yet", systemImage: "film.stack")
            } else if model.largeFileItems.isEmpty {
                ContentUnavailableView("No large files", systemImage: "checkmark.seal")
            } else {
                List(model.largeFileItems) { item in
                    HStack(alignment: .top) {
                        Toggle(isOn: Binding(
                            get: { item.isSelected },
                            set: { model.toggleLargeFile(item, selected: $0) }
                        )) { EmptyView() }
                        .labelsHidden()
                        .accessibilityLabel(item.displayName)

                        VStack(alignment: .leading, spacing: 2) {
                            Text(item.displayName).font(.body.weight(.medium)).lineLimit(1)
                            Text(item.path).font(.caption).foregroundStyle(.secondary).lineLimit(1)
                            HStack(spacing: 6) {
                                Text(item.formattedSize).font(.caption).monospacedDigit()
                                if item.isVideo { Text("Video").font(.caption2).foregroundStyle(.purple) }
                                if item.isImage { Text("Image").font(.caption2).foregroundStyle(.teal) }
                            }
                        }
                        Spacer()
                        if (item.isVideo || item.isImage) && model.compressingPath != item.path {
                            Button("Convert") {
                                if item.isVideo { model.compressVideo(item) }
                                else { model.compressImage(item) }
                            }
                            .controlSize(.small)
                            .disabled(model.compressingPath != nil)
                        } else if model.compressingPath == item.path {
                            ProgressView().controlSize(.small)
                        }
                    }
                    .contextMenu {
                        Button("Reveal in Finder") { model.revealInFinder(item.path) }
                    }
                }
            }
        }
        .navigationTitle("Large Files")
        .onAppear {
            if !model.hasScannedLargeFiles { model.scanLargeFiles() }
        }
        .onDisappear { model.cancelCompression() }
    }

    private var thresholdLabel: String {
        let mb = UserDefaults.standard.object(forKey: "SafeUninstaller.largeFileThresholdMB") as? Int ?? 100
        return "\(mb) MB"
    }
}

struct LargeFilesDetailView: View {
    @EnvironmentObject private var model: AppViewModel

    var body: some View {
        VStack(alignment: .leading, spacing: 16) {
            Text("Convert & clean").font(.title2.bold())
            Text("Use Convert on media rows. Formats below apply to the next conversion.")
                .foregroundStyle(.secondary)

            GroupBox("Formats") {
                VStack(alignment: .leading, spacing: 8) {
                    Button("Video: \(model.videoOutputFormat.displayName)") {
                        cycle(&model.videoOutputFormat, through: VideoOutputFormat.allCases)
                    }
                    .buttonStyle(.plain)
                    Text(model.videoOutputFormat.detail).font(.caption).foregroundStyle(.secondary)

                    Button("Quality: \(model.videoQualityPreset.displayName)") {
                        cycle(&model.videoQualityPreset, through: VideoQualityPreset.allCases)
                    }
                    .buttonStyle(.plain)
                    Text(model.videoQualityPreset.detail).font(.caption).foregroundStyle(.secondary)

                    Button("Image: \(model.imageOutputFormat.displayName)") {
                        cycle(&model.imageOutputFormat, through: ImageOutputFormat.allCases)
                    }
                    .buttonStyle(.plain)
                    Text(model.imageOutputFormat.detail).font(.caption).foregroundStyle(.secondary)
                }
                .frame(maxWidth: .infinity, alignment: .leading)
            }

            GroupBox {
                VStack(alignment: .leading, spacing: 8) {
                    Text("\(model.selectedLargeFiles.count) selected · \(ByteCountFormatter.string(fromByteCount: model.selectedLargeFiles.reduce(0) { $0 + $1.byteSize }, countStyle: .file))")
                    Button("Move Selected to Trash…", role: .destructive) {
                        model.requestLargeFileRemoval()
                    }
                    .disabled(model.selectedLargeFiles.isEmpty || model.isRemoving)
                }
                .frame(maxWidth: .infinity, alignment: .leading)
            }
            Spacer()
        }
        .padding()
    }

    private func cycle<T: Equatable>(_ value: inout T, through cases: [T]) {
        guard let idx = cases.firstIndex(of: value) else { return }
        value = cases[(idx + 1) % cases.count]
    }
}

struct CachesPane: View {
    @EnvironmentObject private var model: AppViewModel

    var body: some View {
        VStack(spacing: 0) {
            HStack {
                Text("~/Library/Caches — Apple system caches are hidden.")
                    .font(.caption)
                    .foregroundStyle(.secondary)
                Spacer()
                Button("Select All") { model.selectAllCaches(true) }
                Button("Deselect") { model.selectAllCaches(false) }
                Button("Scan") { model.scanCaches() }
                    .disabled(model.isScanningCaches)
            }
            .padding(8)

            if model.isScanningCaches {
                ProgressView("Scanning…").frame(maxWidth: .infinity, maxHeight: .infinity)
            } else if !model.hasScannedCaches {
                ContentUnavailableView("Not scanned yet", systemImage: "internaldrive")
            } else if model.cacheItems.isEmpty {
                ContentUnavailableView("No clearable caches", systemImage: "checkmark.seal")
            } else {
                List(model.cacheItems) { item in
                    HStack(alignment: .top) {
                        Toggle(isOn: Binding(
                            get: { item.isSelected },
                            set: { model.toggleCache(item, selected: $0) }
                        )) { EmptyView() }
                        .labelsHidden()
                        .accessibilityLabel(item.displayName)

                        VStack(alignment: .leading, spacing: 2) {
                            Text(item.displayName).font(.body.weight(.medium))
                            if let owner = item.inferredOwner {
                                Text(owner).font(.caption).foregroundStyle(.secondary)
                            }
                            Text(item.note).font(.caption2).foregroundStyle(.tertiary)
                        }
                        Spacer()
                        Text(item.formattedSize).font(.caption).monospacedDigit()
                    }
                    .contextMenu {
                        Button("Reveal in Finder") { model.revealInFinder(item.path) }
                    }
                }
            }
        }
        .navigationTitle("Caches")
        .onAppear {
            if !model.hasScannedCaches { model.scanCaches() }
        }
    }
}

struct CachesDetailView: View {
    @EnvironmentObject private var model: AppViewModel

    var body: some View {
        VStack(alignment: .leading, spacing: 16) {
            Text("Caches").font(.title2.bold())
            Text("Clearing caches is usually safe. Apps rebuild them as needed. Nothing is deleted until you confirm.")
                .foregroundStyle(.secondary)

            GroupBox {
                VStack(alignment: .leading, spacing: 8) {
                    Text("\(model.selectedCaches.count) selected · \(ByteCountFormatter.string(fromByteCount: model.selectedCaches.reduce(0) { $0 + $1.byteSize }, countStyle: .file))")
                    Button("Move Selected to Trash…", role: .destructive) {
                        model.requestCacheRemoval()
                    }
                    .disabled(model.selectedCaches.isEmpty || model.isRemoving)
                }
                .frame(maxWidth: .infinity, alignment: .leading)
            }
            Spacer()
        }
        .padding()
    }
}

struct DeveloperPane: View {
    @EnvironmentObject private var model: AppViewModel

    var body: some View {
        VStack(spacing: 0) {
            HStack {
                Text("Nothing is pre-selected. Review consequences before removing.")
                    .font(.caption)
                    .foregroundStyle(.secondary)
                Spacer()
                Button("Select All") { model.selectAllDevItems(true) }
                    .disabled(model.devItems.isEmpty)
                Button("Scan") { model.scanDevDependencies() }
                    .disabled(model.isScanningDev)
            }
            .padding(8)

            if model.isScanningDev {
                ProgressView("Scanning…").frame(maxWidth: .infinity, maxHeight: .infinity)
            } else if !model.hasScannedDev {
                ContentUnavailableView("Not scanned yet", systemImage: "hammer")
            } else if model.devItems.isEmpty && model.cliCleanupActions.filter(\.isAvailable).isEmpty {
                ContentUnavailableView("Nothing found", systemImage: "checkmark.seal", description: Text("No Xcode, package, or project dependency folders turned up."))
            } else {
                List {
                    if !model.cliCleanupActions.isEmpty {
                        Section("Official CLI cleanups") {
                            ForEach(model.cliCleanupActions) { action in
                                HStack(alignment: .top) {
                                    VStack(alignment: .leading, spacing: 2) {
                                        Text(action.title).font(.body.weight(.medium))
                                        Text(action.detail).font(.caption).foregroundStyle(.secondary)
                                    }
                                    Spacer()
                                    Button("Run…") { model.requestCLICleanup(action) }
                                        .disabled(!action.isAvailable || model.isRunningCLICleanup)
                                        .help(action.isAvailable ? action.detail : "Tool not found on PATH")
                                }
                            }
                        }
                    }

                    ForEach(DevDependencyGroup.allCases) { group in
                        let items = model.devItems.filter { $0.group == group }
                        if !items.isEmpty {
                            Section(group.displayName) {
                                ForEach(items) { item in
                                    HStack(alignment: .top) {
                                        Toggle(isOn: Binding(
                                            get: { item.isSelected },
                                            set: { model.toggleDevItem(item, selected: $0) }
                                        )) { EmptyView() }
                                        .labelsHidden()
                                        .accessibilityLabel(item.displayName)

                                        VStack(alignment: .leading, spacing: 2) {
                                            HStack {
                                                Text(item.displayName).font(.body.weight(.medium))
                                                if item.isHighRisk {
                                                    Text("caution")
                                                        .font(.caption2)
                                                        .padding(.horizontal, 5)
                                                        .padding(.vertical, 1)
                                                        .background(Color.orange.opacity(0.2))
                                                        .clipShape(RoundedRectangle(cornerRadius: 3))
                                                }
                                            }
                                            Text(item.path).font(.caption2).foregroundStyle(.tertiary).lineLimit(1)
                                            Text(item.consequence).font(.caption).foregroundStyle(.secondary)
                                        }
                                        Spacer()
                                        Text(item.formattedSize).font(.caption).monospacedDigit()
                                    }
                                    .contextMenu {
                                        Button("Reveal in Finder") { model.revealInFinder(item.path) }
                                    }
                                }
                            }
                        }
                    }
                }
            }
        }
        .navigationTitle("Developer")
        .onAppear {
            model.refreshCLICleanupActions()
            if !model.hasScannedDev { model.scanDevDependencies() }
        }
    }
}

struct DeveloperDetailView: View {
    @EnvironmentObject private var model: AppViewModel

    var body: some View {
        VStack(alignment: .leading, spacing: 16) {
            Text("Developer Dependencies").font(.title2.bold())
            Text("Remove Xcode leftovers, package manager caches, and per-project folders like node_modules — only when you choose them. Use official CLI cleanups for brew/npm/simulators when those tools are installed.")
                .foregroundStyle(.secondary)

            if model.isRunningCLICleanup {
                ProgressView("Running CLI cleanup…")
            }

            if let summary = model.cliCleanupSummary {
                GroupBox("Last CLI result") {
                    Text(summary)
                        .font(.caption.monospaced())
                        .textSelection(.enabled)
                        .frame(maxWidth: .infinity, alignment: .leading)
                }
            }

            if model.selectedDevItems.contains(where: \.isHighRisk) {
                Text("Your selection includes high-risk items (e.g. Docker or Archives). Double-check before continuing.")
                    .font(.callout)
                    .foregroundStyle(.orange)
            }

            GroupBox {
                VStack(alignment: .leading, spacing: 8) {
                    Text("\(model.selectedDevItems.count) selected · \(ByteCountFormatter.string(fromByteCount: model.selectedDevItems.reduce(0) { $0 + $1.byteSize }, countStyle: .file))")
                    Button("Move Selected to Trash…", role: .destructive) {
                        model.requestDevRemoval()
                    }
                    .disabled(model.selectedDevItems.isEmpty || model.isRemoving)
                }
                .frame(maxWidth: .infinity, alignment: .leading)
            }
            Spacer()
        }
        .padding()
    }
}

struct CLICleanupConfirmSheet: View {
    @EnvironmentObject private var model: AppViewModel

    var body: some View {
        VStack(alignment: .leading, spacing: 16) {
            Text("Run cleanup?").font(.title2.bold())
            if let action = model.pendingCLIAction {
                Text(action.title).font(.headline)
                Text(action.detail).foregroundStyle(.secondary)
                Text("This runs a local command on your Mac. Nothing is sent over the network by this app.")
                    .font(.caption)
                    .foregroundStyle(.secondary)
            }
            HStack {
                Button("Cancel", role: .cancel) {
                    model.showCLICleanupConfirm = false
                    model.pendingCLIAction = nil
                }
                .keyboardShortcut(.cancelAction)
                Spacer()
                Button("Run") { model.performCLICleanup() }
                    .keyboardShortcut(.defaultAction)
                    .disabled(model.pendingCLIAction == nil)
            }
        }
        .padding(24)
        .frame(width: 440)
    }
}
