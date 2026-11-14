import SwiftUI
import XCTest

@testable import tauri_plugin_crowbar_chrome

final class ChromePaletteTests: XCTestCase {
  func testPaletteWithUnparsableColourIsNil() throws {
    let state = try decode(
      PaletteState.self,
      ##"{"surface":"#000","raised":"nope","text":"#fff","secondaryText":"#888"}"##
    )
    XCTAssertNil(ChromePalette(state))
  }

  func testPaletteCarriesTheSidebarPaletteWhenTheThemeHasOne() throws {
    let state = try decode(
      PaletteState.self,
      ##"""
      {"surface":"#fcfcfa","raised":"#f1f0ea","text":"#1b1b19","secondaryText":"#666",
       "sidebar":{"text":"oklch(0.94 0.008 106.7)","brand":"#98a86a","brandText":"#1b1b19"}}
      """##
    )
    XCTAssertNotNil(ChromePalette(state)?.sidebar)
  }

  func testPaletteWithoutSidebarPaletteKeepsThePlainChrome() throws {
    let plain = try decode(
      PaletteState.self,
      ##"{"surface":"#fff","raised":"#eee","text":"#000","secondaryText":"#666","sidebar":null}"##
    )
    XCTAssertNotNil(ChromePalette(plain))
    XCTAssertNil(ChromePalette(plain)?.sidebar)
    let unparsable = try decode(
      PaletteState.self,
      ##"{"surface":"#fff","raised":"#eee","text":"#000","secondaryText":"#666","sidebar":{"text":"nope","brand":"#98a86a","brandText":"#000"}}"##
    )
    XCTAssertNil(ChromePalette(unparsable)?.sidebar)
  }

  func testBrandFollowsTheColourScheme() throws {
    let state = try decode(
      PaletteState.self,
      ##"{"surface":"#fff","raised":"#eee","text":"#000","secondaryText":"#666","brandLight":"#606c38","brandDark":"#98a86a"}"##
    )
    let palette = try XCTUnwrap(ChromePalette(state))
    XCTAssertEqual(palette.brand(.light), Color(css: "#606c38"))
    XCTAssertEqual(palette.brand(.dark), Color(css: "#98a86a"))
  }
}
