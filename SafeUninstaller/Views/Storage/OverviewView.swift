import SwiftUI
import UninstallerCore

struct OverviewView: View {
    @EnvironmentObject private var model: AppViewModel

    var body: some View {
        ScrollView {
            VStack(alignment: .leading, spacing: 20) {
                Text("Storage").font(.title2.bold())
                Text("See where space goes, then free it safely. Items go to Trash first — empty Trash to reclaim space.")
                    .foregroundStyle(.secondary)

                if model.isLoadingOverview && model.storageOverview == nil {
                    ProgressView("Measuring…")
                        .frame(maxWidth: .infinity)
                        .padding(.vertical, 40)
                } else if let overview = model.storageOverview {
                    gauge(overview)
                    categoryList(overview)
                    emptyTrashCard
                    quickLinks
                } else {
                    Button("Measure Storage") { model.refreshStorageOverview() }
                        .buttonStyle(.borderedProminent)
                }
            }
            .padding()
        }
        .navigationTitle("Overview")
        .toolbar {
            ToolbarItem {
                Button {
                    model.refreshStorageOverview()
                } label: {
                    Image(systemName: "arrow.clockwise")
                }
                .help("Refresh storage measurements")
            }
        }
    }

    private func gauge(_ overview: StorageOverview) -> some View {
        VStack(alignment: .leading, spacing: 8) {
            HStack {
                Text("\(overview.formattedAvailable) free")
                    .font(.title3.weight(.semibold))
                Spacer()
                Text("of \(overview.formattedTotal)")
                    .foregroundStyle(.secondary)
            }
            ProgressView(value: overview.usedFraction)
                .tint(overview.usedFraction > 0.9 ? .red : (overview.usedFraction > 0.75 ? .orange : .accentColor))
            Text("\(Int(overview.usedFraction * 100))% used")
                .font(.caption)
                .foregroundStyle(.secondary)
        }
        .padding()
        .background(.regularMaterial, in: RoundedRectangle(cornerRadius: 12))
    }

    private func categoryList(_ overview: StorageOverview) -> some View {
        VStack(alignment: .leading, spacing: 10) {
            Text("Where space goes").font(.headline)
            let maxSize = max(overview.categories.map(\.byteSize).max() ?? 1, 1)
            ForEach(overview.categories) { cat in
                Button {
                    jump(to: cat.kind)
                } label: {
                    HStack {
                        Image(systemName: cat.kind.systemImage)
                            .frame(width: 20)
                        Text(cat.kind.displayName)
                        Spacer()
                        Text(cat.formattedSize)
                            .foregroundStyle(.secondary)
                            .monospacedDigit()
                    }
                }
                .buttonStyle(.plain)
                ProgressView(value: Double(cat.byteSize) / Double(maxSize))
                    .tint(.secondary)
            }
        }
        .padding()
        .background(.regularMaterial, in: RoundedRectangle(cornerRadius: 12))
    }

    private var emptyTrashCard: some View {
        HStack {
            VStack(alignment: .leading, spacing: 4) {
                Text("Trash").font(.headline)
                Text(ByteCountFormatter.string(fromByteCount: model.trashBytes, countStyle: .file) + " waiting to be freed")
                    .font(.caption)
                    .foregroundStyle(.secondary)
            }
            Spacer()
            Button("Empty Trash…") { model.requestEmptyTrash() }
                .disabled(model.trashBytes == 0)
            Button("Open Trash") { model.openTrash() }
        }
        .padding()
        .background(.regularMaterial, in: RoundedRectangle(cornerRadius: 12))
    }

    private var quickLinks: some View {
        VStack(alignment: .leading, spacing: 8) {
            Text("Free space").font(.headline)
            LazyVGrid(columns: [GridItem(.adaptive(minimum: 140), spacing: 10)], spacing: 10) {
                link("Applications", "Uninstall completely", .applications, "square.grid.2x2")
                link("Leftovers", "Orphaned app data", .leftovers, "doc.badge.questionmark")
                link("Large Files", "Videos & big files", .largeFiles, "film.stack")
                link("Caches", "App caches", .caches, "internaldrive")
                link("Developer", "Xcode & deps", .developer, "hammer")
            }
        }
    }

    private func link(_ title: String, _ subtitle: String, _ item: AppViewModel.SidebarItem, _ image: String) -> some View {
        Button {
            model.sidebar = item
        } label: {
            VStack(alignment: .leading, spacing: 6) {
                Image(systemName: image)
                Text(title).font(.callout.weight(.semibold))
                Text(subtitle).font(.caption2).foregroundStyle(.secondary)
            }
            .frame(maxWidth: .infinity, alignment: .leading)
            .padding(12)
            .background(Color.secondary.opacity(0.08), in: RoundedRectangle(cornerRadius: 10))
        }
        .buttonStyle(.plain)
    }

    private func jump(to kind: StorageCategoryKind) {
        switch kind {
        case .applications: model.sidebar = .applications
        case .caches: model.sidebar = .caches
        case .downloads, .movies: model.sidebar = .largeFiles
        case .developer: model.sidebar = .developer
        case .trash: model.openTrash()
        }
    }
}
