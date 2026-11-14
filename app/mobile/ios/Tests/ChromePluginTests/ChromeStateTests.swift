import XCTest

@testable import tauri_plugin_crowbar_chrome

final class ChromeStateTests: XCTestCase {
  func testChromeStateOmittedPartsAreNil() throws {
    let state = try decode(ChromeState.self, "{}")
    XCTAssertNil(state.welcome)
    XCTAssertNil(state.connect)
    XCTAssertNil(state.headers)
    XCTAssertNil(state.ack)
    XCTAssertNil(state.reveal)
    XCTAssertNil(state.settings)
  }

  func testChromeStateFull() throws {
    let state = try decode(
      ChromeState.self,
      """
      {"accent":"#fff","welcome":{"visible":true,"label":"Connect"},
       "connect":{"open":true,"stage":"fingerprint","name":"n","address":"a","code":"",
         "fingerprint":"ab:cd","busy":false,"error":null},
       "headers":{"chat":[],"ide":[{"id":"files","icon":"FolderSimple","side":"trailing",
         "label":"Files","enabled":true}]},
       "reveal":"chatList","ack":4,"settings":{"open":true,"theme":"crowbar","themes":[{"id":"crowbar","name":"Crowbar"}],"pages":[{"id":"hosts","label":"Hosts","icon":"HardDrives"}]},"appearance":"dark","hostPlace":"local","hostStatus":"needs-repair",
       "palette":{"surface":"#000","raised":"#111","text":"#fff","secondaryText":"#888","brandLight":"#606c38","brandDark":"#98a86a"}}
      """
    )
    XCTAssertEqual(state.connect?.stage, .fingerprint)
    XCTAssertNil(state.connect?.error)
    XCTAssertEqual(state.headers?.chat, [])
    XCTAssertEqual(state.headers?.ide.first?.intent, nil)
    XCTAssertEqual(state.ack, 4)
    XCTAssertEqual(state.reveal, .chatList)
    XCTAssertEqual(Reveal.none.screen, .chat)
    XCTAssertEqual(Reveal.files.screen, .files)
    XCTAssertEqual(state.hostPlace, "local")
    XCTAssertEqual(state.hostStatus, "needs-repair")
    XCTAssertEqual(state.settings?.theme, "crowbar")
    XCTAssertEqual(state.settings?.themes, [ThemeChoice(id: "crowbar", name: "Crowbar")])
    XCTAssertEqual(state.settings?.pages.first?.icon, "HardDrives")
    XCTAssertNotNil(state.palette.flatMap(ChromePalette.init)?.brandLight)
    XCTAssertNotNil(state.palette.flatMap(ChromePalette.init)?.brandDark)
  }

  func testUnknownRevealAndStageAreRejected() {
    XCTAssertThrowsError(try decode(Reveal.self, #""sideways""#))
    XCTAssertThrowsError(try decode(ConnectStage.self, #""done""#))
  }
}
