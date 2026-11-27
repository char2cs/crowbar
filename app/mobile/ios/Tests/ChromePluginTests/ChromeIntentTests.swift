import XCTest

@testable import tauri_plugin_crowbar_chrome

final class ChromeIntentTests: XCTestCase {
  func testIntentPayloadsCarryTheirType() {
    XCTAssertEqual(ChromeIntent.tap("files").payload["type"] as? String, "tap")
    XCTAssertEqual(ChromeIntent.tap("files").payload["id"] as? String, "files")
    let address = ChromeIntent.submitAddress(name: "n", address: "a").payload
    XCTAssertEqual(address["type"] as? String, "submitAddress")
    XCTAssertEqual(address["address"] as? String, "a")
  }
}
