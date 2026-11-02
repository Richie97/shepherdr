import XCTest

/// Taps through the demo agents (no hub needed). Works in both layouts: a
/// pushed stack on compact phones, list + detail side by side on wide ones
/// (unfolded Duo, landscape Pro Max, iPad).
final class AgentsUITests: XCTestCase {
    override func setUp() { continueAfterFailure = false }

    @MainActor
    func testTappingAnAgentOpensIt() {
        let app = XCUIApplication()
        app.launchArguments = ["-demo", "YES"]
        app.launch()

        let row = app.staticTexts["Add rate limiting to /v2/orders"]
        XCTAssertTrue(row.waitForExistence(timeout: 10))
        row.tap()

        // The detail's composer is named after the agent (t2 is codex).
        XCTAssertTrue(app.textFields["Message codex"].waitForExistence(timeout: 5), "agent detail didn't open")

        // Compact: go back to the list. Regular: the list is still on screen.
        // iOS 26 labels the back button with the list's title; iOS 27 drops
        // the label and keeps only the identifier.
        let back = [app.buttons["BackButton"], app.navigationBars.buttons["Agents"]].first { $0.exists }
        back?.tap()

        let other = app.staticTexts["Fix flaky retry test"]
        XCTAssertTrue(other.waitForExistence(timeout: 5), "agent list isn't showing")
        other.tap()
        XCTAssertTrue(app.textFields["Message claude-code"].waitForExistence(timeout: 5), "second agent didn't open")
    }
}
