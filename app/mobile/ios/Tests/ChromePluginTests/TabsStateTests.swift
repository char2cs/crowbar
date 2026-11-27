import XCTest

@testable import tauri_plugin_crowbar_chrome

final class TabsStateTests: XCTestCase {
  func testTabsState() throws {
    let state = try decode(
      TabsState.self,
      """
      {"tabs":[{"id":"a","title":"main.rs","kind":"file","path":"src/main.rs","active":true,
        "dirty":false,"closable":true},
        {"id":"b","title":"zsh","kind":"terminal","active":false,"dirty":true,"closable":false}],
       "newTabActions":[{"id":"term","title":"Terminal","icon":"TerminalWindow"}]}
      """
    )
    XCTAssertEqual(state.tabs.map(\.id), ["a", "b"])
    XCTAssertEqual(state.tabs[0].path, "src/main.rs")
    XCTAssertNil(state.tabs[1].path)
    XCTAssertTrue(state.tabs[1].dirty)
    XCTAssertEqual(state.newTabActions.first?.icon, "TerminalWindow")
  }

  func testTabsStateRejectsMissingField() {
    XCTAssertThrowsError(try decode(TabsState.self, #"{"tabs":[]}"#))
  }
}
