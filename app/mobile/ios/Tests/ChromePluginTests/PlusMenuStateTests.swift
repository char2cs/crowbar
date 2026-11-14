import XCTest

@testable import tauri_plugin_crowbar_chrome

final class PlusMenuStateTests: XCTestCase {
  func testPlusMenuStateSplitsGroupsAtTheDivider() throws {
    let state = try decode(
      PlusMenuState.self,
      """
      {"open":true,"title":"Add","items":[
        {"id":"photos","title":"Photo library","icon":"Images","hint":null,"divider":false},
        {"id":"drawing","title":"New drawing","icon":"PencilSimpleLine","hint":null,"divider":false},
        {"id":"mention","title":"Mention a file","icon":"At","hint":"@","divider":true},
        {"id":"command","title":"Run a command","icon":"TerminalWindow","hint":"/","divider":false}]}
      """
    )
    XCTAssertEqual(
      state.groups.map { $0.map(\.id) },
      [["photos", "drawing"], ["mention", "command"]]
    )
    for item in state.items { XCTAssertNotNil(CrowbarIcon(named: item.icon), item.icon) }
  }
}
