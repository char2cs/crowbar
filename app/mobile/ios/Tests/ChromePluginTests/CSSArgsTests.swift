import XCTest

@testable import tauri_plugin_crowbar_chrome

final class CSSArgsTests: XCTestCase {
  private struct ParseCase {
    let name: String
    let input: String
    let parts: [String]
    let alpha: Double
  }

  private static func table(_ items: ParseCase...) -> [ParseCase] { items }

  private static let parseCases = table(
    ParseCase(
      name: "comma parts are opaque",
      input: "255, 128, 0",
      parts: ["255", "128", "0"],
      alpha: 1
    ),
    ParseCase(
      name: "slash introduces alpha",
      input: "0 255 0 / 25%",
      parts: ["0", "255", "0"],
      alpha: 0.25
    ),
    ParseCase(
      name: "legacy fourth number is alpha",
      input: "255, 0, 0, 0.5",
      parts: ["255", "0", "0"],
      alpha: 0.5
    ),
    ParseCase(
      name: "leading keyword keeps all parts",
      input: "srgb 0 1 0",
      parts: ["srgb", "0", "1", "0"],
      alpha: 1
    )
  )

  func testPartsAndAlphaFollowTheSyntax() {
    for item in Self.parseCases {
      let args = CSSArgs(item.input)
      XCTAssertEqual(args.parts, item.parts, item.name)
      XCTAssertEqual(args.alpha, item.alpha, accuracy: 0.0001, item.name)
    }
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
