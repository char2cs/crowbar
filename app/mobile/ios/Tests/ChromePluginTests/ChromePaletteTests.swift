import SwiftUI
import XCTest

@testable import tauri_plugin_crowbar_chrome

final class ChromePaletteTests: XCTestCase {
  private struct PaletteCase {
    let name: String
    let json: String
    let hasPalette: Bool
    let hasSidebar: Bool
  }

  private let paletteCases = [
    PaletteCase(
      name: "an unparsable colour gives no palette",
      json: ##"{"surface":"#000","raised":"nope","text":"#fff","secondaryText":"#888"}"##,
      hasPalette: false,
      hasSidebar: false
    ),
    PaletteCase(
      name: "the sidebar palette is carried when the theme has one",
      json: ##"""
        {"surface":"#fcfcfa","raised":"#f1f0ea","text":"#1b1b19","secondaryText":"#666",
         "sidebar":{"text":"oklch(0.94 0.008 106.7)","brand":"#98a86a","brandText":"#1b1b19"}}
        """##,
      hasPalette: true,
      hasSidebar: true
    ),
    PaletteCase(
      name: "no sidebar palette keeps the plain chrome",
      json: ##"""
        {"surface":"#fff","raised":"#eee","text":"#000","secondaryText":"#666",
         "sidebar":null}
        """##,
      hasPalette: true,
      hasSidebar: false
    ),
    PaletteCase(
      name: "an unparsable sidebar colour drops only the sidebar",
      json: ##"""
        {"surface":"#fff","raised":"#eee","text":"#000","secondaryText":"#666",
         "sidebar":{"text":"nope","brand":"#98a86a","brandText":"#000"}}
        """##,
      hasPalette: true,
      hasSidebar: false
    ),
  ]

  func testThePaletteFollowsItsState() throws {
    for item in paletteCases {
      let palette = ChromePalette(try decode(PaletteState.self, item.json))
      XCTAssertEqual(palette != nil, item.hasPalette, item.name)
      XCTAssertEqual(palette?.sidebar != nil, item.hasSidebar, item.name)
    }
  }

  func testBrandFollowsTheColourScheme() throws {
    let state = try decode(
      PaletteState.self,
      ##"""
      {"surface":"#fff","raised":"#eee","text":"#000","secondaryText":"#666",
       "brandLight":"#606c38","brandDark":"#98a86a"}
      """##
    )
    let palette = try XCTUnwrap(ChromePalette(state))
    XCTAssertEqual(palette.brand(.light), Color(css: "#606c38"))
    XCTAssertEqual(palette.brand(.dark), Color(css: "#98a86a"))
  }
}
