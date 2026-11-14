import XCTest

@testable import tauri_plugin_crowbar_chrome

final class SheetPlanTests: XCTestCase {
  private func shown() -> SheetPlan {
    var plan = SheetPlan()
    XCTAssertEqual(plan.update(.open), .present)
    XCTAssertEqual(plan.finished(), .none)
    return plan
  }

  // The web's state push that still says open, arriving after a swipe-down and before the web
  // has handled the dismissal, must not present the sheet again.
  func testStaleOpenAfterUserDismissDoesNotRepresent() {
    var plan = shown()
    plan.dismissedByUser()
    XCTAssertEqual(plan.update(.open), .none)
    XCTAssertEqual(plan.update(.closed), .none)
    XCTAssertEqual(plan.update(.open), .present)
  }

  func testWebCloseDismissesOnce() {
    var plan = shown()
    XCTAssertEqual(plan.update(.closed), .dismiss)
    XCTAssertEqual(plan.update(.closed), .none)
    XCTAssertEqual(plan.finished(), .none)
  }

  // Open and closed faster than the animation: the last word wins once it settles.
  func testFlipsDuringAnimationSettleToTheLastValue() {
    var plan = SheetPlan()
    XCTAssertEqual(plan.update(.open), .present)
    XCTAssertEqual(plan.update(.closed), .none)
    XCTAssertEqual(plan.finished(), .dismiss)
    XCTAssertEqual(plan.update(.open), .none)
    XCTAssertEqual(plan.finished(), .present)
  }

  // Open with no view controller to present over: the plan waits for one, never loops.
  func testOpenWithNoRootWaitsForOne() {
    var plan = SheetPlan()
    XCTAssertEqual(plan.update(.open), .present)
    XCTAssertEqual(plan.noRoot(), .none)
    XCTAssertEqual(plan.update(.open), .present)
    XCTAssertEqual(plan.finished(), .none)
    XCTAssertEqual(plan.update(.open), .none)
  }

  func testClosingWhileWaitingForARootPresentsNothing() {
    var plan = SheetPlan()
    XCTAssertEqual(plan.update(.open), .present)
    XCTAssertEqual(plan.noRoot(), .none)
    XCTAssertEqual(plan.update(.closed), .none)
    XCTAssertEqual(plan.update(.open), .present)
  }
}
