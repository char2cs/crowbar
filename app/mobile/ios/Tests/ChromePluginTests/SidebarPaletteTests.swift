import XCTest

@testable import tauri_plugin_crowbar_chrome

final class SidebarPaletteTests: XCTestCase {
  private struct PaletteCase {
    let text: String
    let brand: String
    let brandText: String
    let isValid: Bool
  }

  private let cases = [
    PaletteCase(text: "#111111", brand: "#ff0000", brandText: "#ffffff", isValid: true),
    PaletteCase(text: "#111", brand: "#f00", brandText: "#fff", isValid: true),
    PaletteCase(text: "not a colour", brand: "#ff0000", brandText: "#ffffff", isValid: false),
    PaletteCase(text: "#111111", brand: "not a colour", brandText: "#ffffff", isValid: false),
    PaletteCase(text: "#111111", brand: "#ff0000", brandText: "not a colour", isValid: false)]

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
