import XCTest

@testable import tauri_plugin_crowbar_chrome

final class CrowbarIconRenderingTests: XCTestCase {
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

  func testCornerButtonIconsAreDrawn() throws {
    let json =
      #"""
      {"chat":[\#
      {"id":"chat-list","icon":"SidebarToggle","side":"leading","label":"a","enabled":true},\#
      {"id":"ide","icon":"Code","side":"trailing","label":"Open IDE","enabled":true}],\#
      "ide":[{"id":"back","icon":"CaretLeft","side":"leading","label":"b","enabled":true},\#
      {"id":"files","icon":"FolderSimple","side":"trailing","label":"c","enabled":true}]}
      """#
    let headers = try JSONDecoder().decode(ChromeHeaders.self, from: Data(json.utf8))
    XCTAssertEqual(headers.chat.last?.icon, "Code")
    for button in headers.chat + headers.ide {
      let icon = try XCTUnwrap(CrowbarIcon(named: button.icon), button.icon)
      XCTAssertFalse(icon.path(in: 20).boundingRect.isEmpty, button.icon)
    }
  }

  private let nameCases: [(name: String, icon: CrowbarIcon?)] =
    [("PencilSimpleLine", .pencilSimpleLine), ("pencil", nil)]

  func testNamesAreTheWebsPhosphorNames() {
    for item in nameCases {
      XCTAssertEqual(CrowbarIcon(named: item.name), item.icon, item.name)
    }
  }
}
