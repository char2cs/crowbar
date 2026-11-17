import XCTest

@testable import tauri_plugin_crowbar_chrome

final class SidebarPaletteTests: XCTestCase {
  private let cases: [(text: String, brand: String, brandText: String, isValid: Bool)] = [
    ("#111111", "#ff0000", "#ffffff", true),
    ("#111", "#f00", "#fff", true),
    ("not a colour", "#ff0000", "#ffffff", false),
    ("#111111", "not a colour", "#ffffff", false),
    ("#111111", "#ff0000", "not a colour", false),
  ]

  func testPaletteExistsOnlyWhenEveryColourParses() {
    for entry in cases {
      let state = SidebarPaletteState(
        text: entry.text,
        brand: entry.brand,
        brandText: entry.brandText
      )
      XCTAssertEqual(SidebarPalette(state) != nil, entry.isValid, entry.text + entry.brand)
    }
  }

  func testEqualStatesGiveEqualPalettes() {
    let state = SidebarPaletteState(text: "#111111", brand: "#ff0000", brandText: "#ffffff")
    XCTAssertEqual(SidebarPalette(state), SidebarPalette(state))
  }
}
