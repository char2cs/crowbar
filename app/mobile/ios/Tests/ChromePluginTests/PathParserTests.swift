import SwiftUI
import XCTest

@testable import tauri_plugin_crowbar_chrome

final class PathParserTests: XCTestCase {
  private struct Case {
    let name: String
    let tokens: [Substring]
    let bounds: CGRect
  }

  private let cases = [
    Case(
      name: "line",
      tokens: ["M1", "2", "L5", "2", "Z"],
      bounds: CGRect(x: 1, y: 2, width: 4, height: 0)
    ),
    Case(
      name: "square",
      tokens: ["M0", "0", "L4", "0", "L4", "4", "L0", "4", "Z"],
      bounds: CGRect(x: 0, y: 0, width: 4, height: 4)
    ),
  ]

  func testPathsBuildFromTheirTokens() {
    for item in cases {
      var parser = PathParser()
      for token in item.tokens { parser.feed(token) }
      parser.flush()
      XCTAssertEqual(parser.path.boundingRect, item.bounds, item.name)
    }
  }
}
