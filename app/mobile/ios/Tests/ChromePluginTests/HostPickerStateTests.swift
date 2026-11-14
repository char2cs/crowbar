import XCTest

@testable import tauri_plugin_crowbar_chrome

final class HostPickerStateTests: XCTestCase {
  func testHostPickerState() throws {
    let state = try decode(
      HostPickerState.self,
      """
      {"open":true,"hosts":[{"id":"h1","name":"lab","endpoint":"https://lab:7777","state":"up"}],
       "revoking":["h1"],"shownHost":"h1"}
      """
    )
    XCTAssertTrue(state.open)
    XCTAssertEqual(state.list.hosts.map(\.name), ["lab"])
    XCTAssertEqual(state.list.revoking, ["h1"])
    XCTAssertEqual(state.list.shownHost, "h1")
  }
}
