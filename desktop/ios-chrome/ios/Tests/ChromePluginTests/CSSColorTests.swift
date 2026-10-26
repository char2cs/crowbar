import SwiftUI
import XCTest

@testable import tauri_plugin_crowbar_chrome

final class CSSColorTests: XCTestCase {
  private func rgba(_ css: String, file: StaticString = #filePath, line: UInt = #line)
    -> [Float]?
  {
    guard let color = Color(css: css) else { return nil }
    let r = color.resolve(in: EnvironmentValues())
    return [r.red, r.green, r.blue, r.opacity]
  }

  private func assertColor(
    _ css: String, _ expected: [Float], accuracy: Float = 0.01,
    file: StaticString = #filePath, line: UInt = #line
  ) {
    guard let got = rgba(css) else { return XCTFail("\(css) did not parse", file: file, line: line) }
    for (a, b) in zip(got, expected) {
      XCTAssertEqual(a, b, accuracy: accuracy, css, file: file, line: line)
    }
  }

  func testHexForms() {
    assertColor("#fff", [1, 1, 1, 1])
    assertColor("#f008", [1, 0, 0, 0.533])
    assertColor("#ff0000", [1, 0, 0, 1])
    assertColor("  #00FF0080 ", [0, 1, 0, 0.502])
  }

  func testRejectsMalformedHex() {
    XCTAssertNil(Color(css: "#12"))
    XCTAssertNil(Color(css: "#12345"))
    XCTAssertNil(Color(css: "#gggggg"))
  }

  func testRgbCommaAndSlashSyntax() {
    assertColor("rgb(255, 0, 0)", [1, 0, 0, 1])
    assertColor("rgba(0, 0, 255, 0.5)", [0, 0, 1, 0.5])
    assertColor("rgb(0 255 0 / 25%)", [0, 1, 0, 0.25])
    assertColor("rgb(100% 0% 0%)", [1, 0, 0, 1])
  }

  func testColorFunctionSpaces() {
    assertColor("color(srgb 0 1 0)", [0, 1, 0, 1])
    // Display-P3 red lies outside sRGB: the extended-sRGB green goes negative.
    guard let p3 = rgba("color(display-p3 1 0 0)") else { return XCTFail("p3 did not parse") }
    XCTAssertGreaterThan(p3[0], 1)
    XCTAssertLessThan(p3[1], 0)
    XCTAssertNil(Color(css: "color(rec2020 1 0 0)"))
  }

  func testOklch() {
    assertColor("oklch(1 0 0)", [1, 1, 1, 1])
    assertColor("oklch(0 0 0)", [0, 0, 0, 1])
    assertColor("oklch(0.628 0.2577 29.23)", [1, 0, 0, 1], accuracy: 0.02)
    assertColor("oklch(50% 0 0 / 0.5)", [0.389, 0.389, 0.389, 0.5], accuracy: 0.02)
  }

  func testRejectsNonColours() {
    XCTAssertNil(Color(css: ""))
    XCTAssertNil(Color(css: "red"))
    XCTAssertNil(Color(css: "hsl(0 100% 50%)"))
    XCTAssertNil(Color(css: "rgb(1, 2)"))
  }
}
