import SwiftUI
import XCTest

@testable import tauri_plugin_crowbar_chrome

final class CSSColorTests: XCTestCase {
  private func rgba(
    _ css: String,
    file: StaticString = #filePath,
    line: UInt = #line
  )
    -> [Float]?
  {
    guard let color = Color(css: css) else { return nil }
    let r = color.resolve(in: EnvironmentValues())
    return [r.red, r.green, r.blue, r.opacity]
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
    for (a, b) in zip(got, expected) {
      XCTAssertEqual(a, b, accuracy: accuracy, css, file: file, line: line)
    }
  }

  private struct AcceptedCase {
    let css: String
    let expected: [Float]
    var accuracy: Float = 0.01
  }

  private let acceptedCases = [
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
    ),
  ]

  private let rejectedCases = [
    "#12", "#12345", "#gggggg", "", "red", "hsl(0 100% 50%)", "rgb(1, 2)", "color(rec2020 1 0 0)",
  ]

  func testAcceptedColoursResolveToTheirChannels() {
    for item in acceptedCases {
      assertColor(item.css, item.expected, accuracy: item.accuracy)
    }
  }

  func testRejectedColoursDoNotParse() {
    for css in rejectedCases {
      XCTAssertNil(Color(css: css), css)
    }
  }

  func testDisplayP3RedLiesOutsideSRGB() {
    // Display-P3 red lies outside sRGB: the extended-sRGB green goes negative.
    guard let p3 = rgba("color(display-p3 1 0 0)") else { return XCTFail("p3 did not parse") }
    XCTAssertGreaterThan(p3[0], 1)
    XCTAssertLessThan(p3[1], 0)
  }
}
