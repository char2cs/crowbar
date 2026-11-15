import XCTest

@testable import tauri_plugin_crowbar_chrome

final class OKLabMatrixTests: XCTestCase {
  private struct Case {
    let name: String
    let lab: [Double]
    let linearRGB: [Double]
  }

  private let cases = [
    Case(name: "black", lab: [0, 0, 0], linearRGB: [0, 0, 0]),
    Case(name: "white", lab: [1, 0, 0], linearRGB: [1, 1, 1]),
  ]

  func testNeutralLabMapsToTheLinearGrey() {
    for item in cases {
      let lms = OKLabMatrix.apply(OKLabMatrix.lmsFromLab, to: item.lab).map { $0 * $0 * $0 }
      let rgb = OKLabMatrix.apply(OKLabMatrix.linearRGBFromLms, to: lms)
      assertClose(rgb, item.linearRGB, item.name)
    }
  }

  private func assertClose(
    _ got: [Double],
    _ want: [Double],
    _ name: String
  ) {
    for (actual, expected) in zip(got, want) {
      XCTAssertEqual(actual, expected, accuracy: 0.001, name)
    }
  }
}
