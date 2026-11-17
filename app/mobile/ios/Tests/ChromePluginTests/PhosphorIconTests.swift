import XCTest

@testable import tauri_plugin_crowbar_chrome

@available(iOS 26.0, *)
final class PhosphorIconTests: XCTestCase {
  func testDefaultsToNoEmphasis() {
    let view = PhosphorIcon(.at)
    XCTAssertEqual(view.icon, .at)
    XCTAssertEqual(view.emphasis, 0)
  }

  func testKeepsTheGivenIconAndEmphasis() {
    let view = PhosphorIcon(.arrowDown, size: 20, emphasis: 1.5)
    XCTAssertEqual(view.icon, .arrowDown)
    XCTAssertEqual(view.emphasis, 1.5)
  }
}
