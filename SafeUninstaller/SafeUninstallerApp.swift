import SwiftUI
import UninstallerCore

@main
struct SafeUninstallerApp: App {
    @StateObject private var model = AppViewModel()

    var body: some Scene {
        WindowGroup {
            ContentView()
                .environmentObject(model)
                .frame(minWidth: 980, minHeight: 640)
                .onAppear {
                    model.bootstrap()
                }
                .onDrop(of: [.fileURL], isTargeted: nil) { providers in
                    model.handleDrop(providers: providers)
                }
        }
        .commands {
            CommandGroup(replacing: .newItem) {}
        }
    }
}
