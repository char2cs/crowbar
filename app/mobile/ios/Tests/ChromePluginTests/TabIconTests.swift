import XCTest

@testable import tauri_plugin_crowbar_chrome

final class TabIconTests: XCTestCase {
  func testTabIcons() {
    XCTAssertEqual(TabIcon.of(kind: "terminal", path: nil), .terminalWindow)
    XCTAssertEqual(TabIcon.of(kind: "file", path: "a/b.SWIFT"), .fileCode)
    XCTAssertEqual(TabIcon.of(kind: "file", path: "notes.md"), .fileText)
    XCTAssertEqual(TabIcon.of(kind: "file", path: "x.unknown"), .file)
    XCTAssertEqual(TabIcon.of(kind: "file", path: nil), .file)
  }
}
