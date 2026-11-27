import SwiftUI
import XCTest

@testable import tauri_plugin_crowbar_chrome

final class CSSColorTests: XCTestCase {
  private func rgba(_ css: String) -> [Float]? {
    guard let color = Color(css: css) else { return nil }
    let resolved = color.resolve(in: EnvironmentValues())
    return [resolved.red, resolved.green, resolved.blue, resolved.opacity]
  }

  private func assertColor(
    _ css: String,
    _ expected: [Float],
    accuracy: Float = 0.01,
    file: StaticString = #filePath,
    line: UInt = #line
  ) {
    guard let got = rgba(css) else {
      return XCTFail("\(css) did not parse", file: file, line: line)
    }
    for (actual, want) in zip(got, expected) {
      XCTAssertEqual(actual, want, accuracy: accuracy, css, file: file, line: line)
    }
  }

  private struct AcceptedCase {
    let css: String
    let expected: [Float]
    var accuracy: Float = 0.01
  }

  private static func table(_ items: AcceptedCase...) -> [AcceptedCase] { items }

  private static let acceptedCases = table(
    AcceptedCase(css: "#fff", expected: [1, 1, 1, 1]),
    AcceptedCase(css: "#f008", expected: [1, 0, 0, 0.533]),
    AcceptedCase(css: "#ff0000", expected: [1, 0, 0, 1]),
    AcceptedCase(css: "  #00FF0080 ", expected: [0, 1, 0, 0.502]),
    AcceptedCase(css: "rgb(255, 0, 0)", expected: [1, 0, 0, 1]),
    AcceptedCase(css: "rgba(0, 0, 255, 0.5)", expected: [0, 0, 1, 0.5]),
    AcceptedCase(css: "rgb(0 255 0 / 25%)", expected: [0, 1, 0, 0.25]),
    AcceptedCase(css: "rgb(100% 0% 0%)", expected: [1, 0, 0, 1]),
    AcceptedCase(css: "color(srgb 0 1 0)", expected: [0, 1, 0, 1]),
    AcceptedCase(css: "oklch(1 0 0)", expected: [1, 1, 1, 1]),
    AcceptedCase(css: "oklch(0 0 0)", expected: [0, 0, 0, 1]),
    AcceptedCase(css: "oklch(0.628 0.2577 29.23)", expected: [1, 0, 0, 1], accuracy: 0.02),
    AcceptedCase(
      css: "oklch(50% 0 0 / 0.5)",
      expected: [0.389, 0.389, 0.389, 0.5],
      accuracy: 0.02
    )
  )

  private let rejectedHex = ["#12", "#12345", "#gggggg", ""]
  private let rejectedFunctions = ["red", "hsl(0 100% 50%)", "rgb(1, 2)", "color(rec2020 1 0 0)"]

  func testAcceptedColoursResolveToTheirChannels() {
    for item in Self.acceptedCases {
      assertColor(item.css, item.expected, accuracy: item.accuracy)
    }
  }

  func testRejectedColoursDoNotParse() {
    for css in rejectedHex + rejectedFunctions {
      XCTAssertNil(Color(css: css), css)
    }
  }

  func testDisplayP3RedLiesOutsideSRGB() {
    guard let wide = rgba("color(display-p3 1 0 0)") else { return XCTFail("p3 did not parse") }
    XCTAssertGreaterThan(wide[0], 1)
    XCTAssertLessThan(wide[1], 0)
  }
}
