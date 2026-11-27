import XCTest

@testable import tauri_plugin_crowbar_chrome

final class ScreenTests: XCTestCase {
  func testScreensOrderLeftToRight() {
    let shuffled: [Screen] = [.files, .chat, .ide, .chatList]
    XCTAssertEqual(shuffled.sorted(), [.chatList, .chat, .ide, .files])
    XCTAssertLessThan(Screen.chat, .ide)
    XCTAssertFalse(Screen.files < .files)
  }
}
