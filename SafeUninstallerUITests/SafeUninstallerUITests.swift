import XCTest

final class SafeUninstallerUITests: XCTestCase {
    var app: XCUIApplication!

    override func setUpWithError() throws {
        continueAfterFailure = false
        app = XCUIApplication()
        app.launchArguments.append("--ui-testing")
        app.launch()
    }

    func testMainWindowAndOverviewSidebar() throws {
        let window = app.windows.firstMatch
        XCTAssertTrue(window.waitForExistence(timeout: 15), "Main window should appear")

        let overview = app.descendants(matching: .any)["sidebar.Overview"]
        if overview.waitForExistence(timeout: 5) {
            overview.click()
        }

        XCTAssertTrue(window.exists)
    }
}
