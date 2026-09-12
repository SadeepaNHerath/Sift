import Foundation
import Sparkle
import UninstallerCore

/// Thin Sparkle wrapper. No-ops when SUFeedURL / SUPublicEDKey are not configured.
@MainActor
final class UpdateController: NSObject, ObservableObject {
    static let shared = UpdateController()

    private var updaterController: SPUStandardUpdaterController?
    private(set) var isConfigured: Bool = false

    func start() {
        let feed = Bundle.main.object(forInfoDictionaryKey: "SUFeedURL") as? String ?? ""
        let key = Bundle.main.object(forInfoDictionaryKey: "SUPublicEDKey") as? String ?? ""
        guard !feed.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty,
              !key.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty else {
            isConfigured = false
            DiagnosticsLog.shared.info("Sparkle updates disabled (SUFeedURL / SUPublicEDKey not set)")
            return
        }
        isConfigured = true
        let controller = SPUStandardUpdaterController(
            startingUpdater: true,
            updaterDelegate: nil,
            userDriverDelegate: nil
        )
        updaterController = controller
        DiagnosticsLog.shared.info("Sparkle updater started")
    }

    func checkForUpdates() {
        guard isConfigured, let updaterController else {
            DiagnosticsLog.shared.info("Check for Updates ignored — feed not configured")
            return
        }
        updaterController.checkForUpdates(nil)
    }
}
