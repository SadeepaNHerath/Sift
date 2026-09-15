import SwiftUI
import UninstallerCore

@main
struct SafeUninstallerApp: App {
    @StateObject private var model = AppViewModel()

    var body: some Scene {
        WindowGroup {
            ContentView()
                .environmentObject(model)
                .onAppear { model.bootstrap() }
                .onDrop(of: [.fileURL], isTargeted: $model.isDropTargeted) { providers in
                    model.handleDrop(providers: providers)
                }
                .frame(minWidth: 900, minHeight: 560)
        }
        .defaultSize(width: 1100, height: 720)
        .commands {
            CommandGroup(replacing: .newItem) {}
            CommandGroup(after: .appInfo) {
                Button("Check for Updates…") {
                    model.checkForUpdatesOrExplain()
                }
            }
            CommandMenu("Clean") {
                Button("Refresh") { model.refreshCurrentModule() }
                    .keyboardShortcut("r", modifiers: [.command])
                Button("Analyze Selected") { model.analyzeSelected() }
                    .keyboardShortcut("a", modifiers: [.command, .shift])
                    .disabled(model.sidebar != .applications || model.selectedApp == nil)
                Divider()
                Button("Select All in Module") { model.selectAllInCurrentModule() }
                    .keyboardShortcut("a", modifiers: [.command])
                Button("Select High Confidence Only") { model.selectOnlyHighConfidence() }
                    .keyboardShortcut("h", modifiers: [.command, .shift])
                    .disabled(model.sidebar != .applications)
                Divider()
                Button("Review & Move to Trash…") { model.requestTrashForCurrentModule() }
                    .keyboardShortcut("t", modifiers: [.command, .shift])
                Button("Empty Trash…") { model.requestEmptyTrash() }
                    .keyboardShortcut("delete", modifiers: [.command, .shift])
            }
            CommandGroup(after: .help) {
                Button("Setup & Permissions…") {
                    model.showSetupSheet = true
                }
                Button("Copy Diagnostics") {
                    model.copyDiagnostics()
                }
            }
        }

        Settings {
            SettingsView()
                .environmentObject(model)
        }
    }
}

struct SettingsView: View {
    @EnvironmentObject private var model: AppViewModel
    @AppStorage("SafeUninstaller.videoQuality") private var videoQualityRaw = VideoQualityPreset.balanced.rawValue
    @AppStorage("SafeUninstaller.imageFormat") private var imageFormatRaw = ImageOutputFormat.automatic.rawValue
    @AppStorage("SafeUninstaller.largeFileThresholdMB") private var largeFileThresholdMB = 100

    var body: some View {
        Form {
            Section("Permissions") {
                LabeledContent("Full Disk Access") {
                    Text(model.hasFDA ? "Granted" : "Not granted")
                        .foregroundStyle(model.hasFDA ? .secondary : .orange)
                }
                Button("Open Full Disk Access Settings…") { model.openFullDiskAccessSettings() }
                Button("Open Automation Settings…") { model.openAutomationPrivacySettings() }
            }
            Section("Large Files") {
                Stepper(value: $largeFileThresholdMB, in: 50...2000, step: 50) {
                    Text("Minimum size: \(largeFileThresholdMB) MB")
                }
                Text("Applied on the next Large Files scan.")
                    .font(.caption)
                    .foregroundStyle(.secondary)
            }
            Section("Compression defaults") {
                Picker("Video quality", selection: $videoQualityRaw) {
                    ForEach(VideoQualityPreset.allCases) { preset in
                        Text(preset.displayName).tag(preset.rawValue)
                    }
                }
                Picker("Image format", selection: $imageFormatRaw) {
                    ForEach(ImageOutputFormat.allCases) { format in
                        Text(format.displayName).tag(format.rawValue)
                    }
                }
            }
            Section("Diagnostics") {
                Button("Copy Diagnostics") { model.copyDiagnostics() }
                Button("Open Log Folder…") { model.openDiagnosticsFolder() }
                Button("Show Setup Sheet…") { model.showSetupSheet = true }
            }
            Section("About") {
                LabeledContent("Product", value: "SafeUninstaller")
                LabeledContent("Repository", value: "Spacefold (GitHub)")
                Text("Ad-hoc builds require Right-click → Open once. Privileged /Library helper needs Developer ID.")
                    .font(.caption)
                    .foregroundStyle(.secondary)
            }
        }
        .formStyle(.grouped)
        .frame(width: 480, height: 420)
        .onAppear { syncPrefsFromStorage() }
        .onChange(of: videoQualityRaw) { _, newValue in
            if let preset = VideoQualityPreset(rawValue: newValue) {
                model.videoQualityPreset = preset
            }
        }
        .onChange(of: imageFormatRaw) { _, newValue in
            if let format = ImageOutputFormat(rawValue: newValue) {
                model.imageOutputFormat = format
            }
        }
    }

    private func syncPrefsFromStorage() {
        if let preset = VideoQualityPreset(rawValue: videoQualityRaw) {
            model.videoQualityPreset = preset
        }
        if let format = ImageOutputFormat(rawValue: imageFormatRaw) {
            model.imageOutputFormat = format
        }
    }
}
