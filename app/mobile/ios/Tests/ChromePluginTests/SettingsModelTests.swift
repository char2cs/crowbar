import XCTest

@testable import tauri_plugin_crowbar_chrome

final class SettingsModelTests: XCTestCase {
  func testApplyHostsReplacesTheList() {
    let model = SettingsModel()
    model.applyHosts(
      #"{"hosts":[{"id":"h1","name":"lab","endpoint":"e","state":"up"}],"revoking":[],"shownHost":"h1"}"#
    )
    XCTAssertEqual(model.hosts.hosts.map(\.id), ["h1"])
    XCTAssertEqual(model.hosts.shownHost, "h1")
  }

  func testUnreadableSnapshotKeepsTheLastList() {
    let model = SettingsModel()
    model.applyHosts(#"{"hosts":[],"revoking":[],"shownHost":"h2"}"#)
    model.applyHosts("not json")
    model.applyHosts(#"{"hosts":[]}"#)
    XCTAssertEqual(model.hosts.shownHost, "h2")
  }
}
