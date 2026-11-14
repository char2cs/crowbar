import XCTest

@testable import tauri_plugin_crowbar_chrome

final class HostListStateTests: XCTestCase {
  func testHostListStateIsTheSettingsHostsSnapshot() throws {
    let list = try decode(
      HostListState.self,
      #"{"hosts":[{"id":"h1","name":"lab","endpoint":"e","state":"down"}],"revoking":[],"shownHost":null}"#
    )
    XCTAssertNil(list.shownHost)
    XCTAssertEqual(list.hosts.first?.state, "down")
  }
}
