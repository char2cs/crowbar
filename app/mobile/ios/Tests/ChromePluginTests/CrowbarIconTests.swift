import XCTest

@testable import tauri_plugin_crowbar_chrome

final class CrowbarIconTests: XCTestCase {
  private let artCases = CrowbarIcon.allCases.map { ($0, $0.art) }

  func testEveryIconHasPathsAndAViewBox() {
    for (icon, art) in artCases {
      XCTAssertFalse(art.paths.isEmpty, icon.rawValue)
      XCTAssertGreaterThan(art.viewBox, 0, icon.rawValue)
    }
  }
}
