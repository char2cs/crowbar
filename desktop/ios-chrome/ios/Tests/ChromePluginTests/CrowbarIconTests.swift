import XCTest

@testable import tauri_plugin_crowbar_chrome

final class CrowbarIconTests: XCTestCase {
  func testEveryIconDrawsInsideItsViewBox() {
    for icon in CrowbarIcon.allCases {
      let bounds = icon.path(in: 100).boundingRect
      XCTAssertFalse(bounds.isEmpty, "\(icon.rawValue) draws nothing")
      let slack = icon.art.stroke * 100 / icon.art.viewBox
      XCTAssertGreaterThanOrEqual(bounds.minX, -slack, icon.rawValue)
      XCTAssertGreaterThanOrEqual(bounds.minY, -slack, icon.rawValue)
      XCTAssertLessThanOrEqual(bounds.maxX, 100 + slack, icon.rawValue)
      XCTAssertLessThanOrEqual(bounds.maxY, 100 + slack, icon.rawValue)
    }
  }

  func testNamesAreTheWebsPhosphorNames() {
    XCTAssertEqual(CrowbarIcon(named: "PencilSimpleLine"), .pencilSimpleLine)
    XCTAssertNil(CrowbarIcon(named: "pencil"))
  }
}
