import SwiftUI
import UninstallerCore

struct LargeFilesPane: View {
    @EnvironmentObject private var model: AppViewModel

    var body: some View {
        VStack(spacing: 0) {
            HStack {
                Text("Files over 100 MB in Downloads, Movies, Desktop, and Documents.")
                    .font(.caption)
                    .foregroundStyle(.secondary)
                Spacer()
                Button("Scan") { model.scanLargeFiles() }
                    .disabled(model.isScanningLargeFiles)
            }
            .padding(8)

            if model.isScanningLargeFiles {
                ProgressView("Scanning…").frame(maxWidth: .infinity, maxHeight: .infinity)
            } else if !model.hasScannedLargeFiles {
                ContentUnavailableView("Not scanned yet", systemImage: "film.stack", description: Text("Find large files and compressible videos."))
            } else if model.largeFileItems.isEmpty {
                ContentUnavailableView("No large files", systemImage: "checkmark.seal", description: Text("Nothing over 100 MB in the usual folders."))
            } else {
                List {
                    ForEach(model.largeFileItems) { item in
                        HStack(alignment: .top) {
                            Toggle(isOn: Binding(
                                get: { item.isSelected },
                                set: { model.toggleLargeFile(item, selected: $0) }
                            )) { EmptyView() }
                            .labelsHidden()
                            .accessibilityLabel(item.displayName)

                            VStack(alignment: .leading, spacing: 2) {
                                Text(item.displayName).font(.body.weight(.medium))
                                Text(item.path).font(.caption2).foregroundStyle(.tertiary).lineLimit(1)
                                HStack {
                                    Text(item.formattedSize)
                                    if item.isVideo {
                                        Text("Video")
                                            .font(.caption2)
                                            .padding(.horizontal, 5)
                                            .padding(.vertical, 1)
                                            .background(Color.purple.opacity(0.15))
                                            .clipShape(RoundedRectangle(cornerRadius: 3))
                                    }
                                }
                                .font(.caption)
                                .foregroundStyle(.secondary)
                            }
                            Spacer()
                            if item.isVideo {
                                if model.compressingPath == item.path {
                                    ProgressView(value: model.compressionProgress)
                                        .frame(width: 80)
                                } else {
                                    Button("Compress") { model.compressVideo(item) }
                                        .disabled(model.compressingPath != nil)
                                        .help("HEVC compress; original goes to Trash")
                                }
                            }
                        }
                        .contextMenu {
                            Button("Reveal in Finder") { model.revealInFinder(item.path) }
                        }
                    }
                }
            }
        }
        .navigationTitle("Large Files")
        .onAppear {
            if !model.hasScannedLargeFiles { model.scanLargeFiles() }
        }
    }
}

struct LargeFilesDetailView: View {
    @EnvironmentObject private var model: AppViewModel

    var body: some View {
        VStack(alignment: .leading, spacing: 16) {
            Text("Large Files & Videos").font(.title2.bold())
            Text("Select files to move to Trash, or compress videos to free space while keeping a smaller copy. Compression uses HEVC when available.")
                .foregroundStyle(.secondary)

            if let path = model.compressingPath {
                VStack(alignment: .leading, spacing: 6) {
                    Text("Compressing \((path as NSString).lastPathComponent)…")
                    ProgressView(value: model.compressionProgress)
                }
                .padding()
                .background(.regularMaterial, in: RoundedRectangle(cornerRadius: 10))
            }

            GroupBox("Selection") {
                VStack(alignment: .leading, spacing: 8) {
                    Text("\(model.selectedLargeFiles.count) selected · \(ByteCountFormatter.string(fromByteCount: model.selectedLargeFiles.reduce(0) { $0 + $1.byteSize }, countStyle: .file))")
                    Text("\(model.largeFileItems.filter(\.isVideo).count) videos can be compressed individually.")
                        .font(.caption)
                        .foregroundStyle(.secondary)
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
                Button("Scan") { model.scanDevDependencies() }
                    .disabled(model.isScanningDev)
            }
            .padding(8)

            if model.isScanningDev {
                ProgressView("Scanning…").frame(maxWidth: .infinity, maxHeight: .infinity)
            } else if !model.hasScannedDev {
                ContentUnavailableView("Not scanned yet", systemImage: "hammer")
            } else if model.devItems.isEmpty {
                ContentUnavailableView("Nothing found", systemImage: "checkmark.seal", description: Text("No Xcode, package, or project dependency folders turned up."))
            } else {
                List {
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
            if !model.hasScannedDev { model.scanDevDependencies() }
        }
    }
}

struct DeveloperDetailView: View {
    @EnvironmentObject private var model: AppViewModel

    var body: some View {
        VStack(alignment: .leading, spacing: 16) {
            Text("Developer Dependencies").font(.title2.bold())
            Text("Remove Xcode leftovers, package manager caches, and per-project folders like node_modules — only when you choose them. Project source files are never touched.")
                .foregroundStyle(.secondary)

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
