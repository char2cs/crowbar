import XCTest

final class WidgetTests: XCTestCase {
  func testTapIncrements() {
    XCTAssertEqual(Widget(taps: 0).tapped().taps, 1)
  }
}
