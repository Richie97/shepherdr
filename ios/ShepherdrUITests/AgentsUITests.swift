import XCTest

/// Taps through the demo agents (no hub needed).
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

        // Pushed: there's a back button to the list.
        let back = app.navigationBars.buttons["Agents"]
        XCTAssertTrue(back.waitForExistence(timeout: 5), "agent detail didn't open")

        // Back, then a different agent.
        back.tap()
        let other = app.staticTexts["Fix flaky retry test"]
        XCTAssertTrue(other.waitForExistence(timeout: 5))
        other.tap()
        XCTAssertTrue(back.waitForExistence(timeout: 5), "second agent didn't open")
    }
}
