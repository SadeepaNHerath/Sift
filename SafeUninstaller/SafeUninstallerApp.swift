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
        }
        .defaultSize(width: 1100, height: 720)
        .commands {
            CommandGroup(replacing: .newItem) {}
            CommandMenu("Clean") {
                Button("Refresh Applications") { model.refreshApps() }
                    .keyboardShortcut("r", modifiers: [.command])
                Button("Analyze Selected") { model.analyzeSelected() }
                    .keyboardShortcut("a", modifiers: [.command, .shift])
                    .disabled(model.selectedApp == nil)
                Divider()
                Button("Select High Confidence Only") { model.selectOnlyHighConfidence() }
                    .keyboardShortcut("h", modifiers: [.command, .shift])
                Button("Review & Move to Trash…") { model.requestUninstall() }
                    .keyboardShortcut("t", modifiers: [.command, .shift])
                    .disabled(model.selectedCount == 0)
            }
        }
    }
}
