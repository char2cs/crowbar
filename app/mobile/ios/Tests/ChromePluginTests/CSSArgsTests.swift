import XCTest

@testable import tauri_plugin_crowbar_chrome

final class CSSArgsTests: XCTestCase {
  func testCommaSeparatedPartsHaveOpaqueAlpha() {
    let args = CSSArgs("255, 128, 0")
    XCTAssertEqual(args.parts, ["255", "128", "0"])
    XCTAssertEqual(args.alpha, 1)
  }

  func testSlashIntroducesTheAlpha() {
    let args = CSSArgs("0 255 0 / 25%")
    XCTAssertEqual(args.parts, ["0", "255", "0"])
    XCTAssertEqual(args.alpha, 0.25, accuracy: 0.0001)
  }

  func testLegacyFourthNumberIsTheAlpha() {
    let args = CSSArgs("255, 0, 0, 0.5")
    XCTAssertEqual(args.parts, ["255", "0", "0"])
    XCTAssertEqual(args.alpha, 0.5, accuracy: 0.0001)
  }

  func testLeadingKeywordKeepsAllFourParts() {
    let args = CSSArgs("srgb 0 1 0")
    XCTAssertEqual(args.parts, ["srgb", "0", "1", "0"])
    XCTAssertEqual(args.alpha, 1)
  }

  func testChannelsScaleAndPercent() {
    let args = CSSArgs("255 50% none")
    XCTAssertEqual(args.channel(0, scale: 255) ?? -1, 1, accuracy: 0.0001)
    XCTAssertEqual(args.channel(1) ?? -1, 0.5, accuracy: 0.0001)
    XCTAssertEqual(args.channel(1, percentOf: 0.4) ?? -1, 0.2, accuracy: 0.0001)
    XCTAssertEqual(args.channel(2), 0)
    XCTAssertNil(args.channel(3))
  }

  func testNumberDropsTheDegreeUnit() {
    let args = CSSArgs("0.6 0.2 29.23deg")
    XCTAssertEqual(args.number(2) ?? -1, 29.23, accuracy: 0.0001)
    XCTAssertNil(args.number(3))
  }
}
