import XCTest

@testable import tauri_plugin_crowbar_chrome

@available(iOS 26.0, *)
final class RowMenuTests: XCTestCase {
  func testRowMenuActionDecoding() {
    let full = RowMenu.action([
      "id": "del", "title": "Delete", "icon": "Trash", "destructive": true,
      "confirm": ["message": "Sure?"],
    ])
    XCTAssertEqual(full?.id, "del")
    XCTAssertEqual(full?.destructive, true)
    XCTAssertEqual(full?.confirm?.title, "Delete")
    XCTAssertEqual(full?.confirm?.message, "Sure?")

    let plain = RowMenu.action(["id": "rename", "title": "Rename"])
    XCTAssertEqual(plain?.destructive, false)
    XCTAssertNil(plain?.confirm)
    XCTAssertNil(plain?.icon)

    XCTAssertNil(RowMenu.action(["id": "x"]))
    XCTAssertNil(RowMenu.action(["title": "x"]))
  }
}
