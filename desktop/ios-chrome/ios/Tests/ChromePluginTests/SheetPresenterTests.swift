import XCTest

@testable import tauri_plugin_crowbar_chrome

final class SheetPresenterTests: XCTestCase {
  private func shown() -> SheetPlan {
    var plan = SheetPlan()
    XCTAssertEqual(plan.update(open: true), .present)
    XCTAssertEqual(plan.finished(), .none)
    return plan
  }

  // The web's state push that still says open, arriving after a swipe-down and before the web
  // has handled the dismissal, must not present the sheet again.
  func testStaleOpenAfterUserDismissDoesNotRepresent() {
    var plan = shown()
    plan.dismissedByUser()
    XCTAssertEqual(plan.update(open: true), .none)
    XCTAssertEqual(plan.update(open: false), .none)
    XCTAssertEqual(plan.update(open: true), .present)
  }

  func testWebCloseDismissesOnce() {
    var plan = shown()
    XCTAssertEqual(plan.update(open: false), .dismiss)
    XCTAssertEqual(plan.update(open: false), .none)
    XCTAssertEqual(plan.finished(), .none)
  }

  // Open and closed faster than the animation: the last word wins once it settles.
  func testFlipsDuringAnimationSettleToTheLastValue() {
    var plan = SheetPlan()
    XCTAssertEqual(plan.update(open: true), .present)
    XCTAssertEqual(plan.update(open: false), .none)
    XCTAssertEqual(plan.finished(), .dismiss)
    XCTAssertEqual(plan.update(open: true), .none)
    XCTAssertEqual(plan.finished(), .present)
  }
}
