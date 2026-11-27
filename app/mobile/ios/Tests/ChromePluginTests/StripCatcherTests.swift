import XCTest

@testable import tauri_plugin_crowbar_chrome

final class StripCatcherTests: XCTestCase {
  func testVoiceOverActivateAndEscapeBothCloseTheDrawer() {
    let catcher = StripCatcher(frame: .zero)
    var closed = 0
    catcher.close = { closed += 1 }
    XCTAssertTrue(catcher.accessibilityActivate())
    XCTAssertTrue(catcher.accessibilityPerformEscape())
    XCTAssertEqual(closed, 2)
  }
}
