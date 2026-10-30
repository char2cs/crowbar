import XCTest

@testable import tauri_plugin_crowbar_chrome

final class PayloadDecodingTests: XCTestCase {
  private func decode<T: Decodable>(_ type: T.Type, _ json: String) throws -> T {
    try JSONDecoder().decode(type, from: Data(json.utf8))
  }

  func testTabsState() throws {
    let state = try decode(
      TabsState.self,
      """
      {"tabs":[{"id":"a","title":"main.rs","kind":"file","path":"src/main.rs","active":true,
        "dirty":false,"closable":true},
        {"id":"b","title":"zsh","kind":"terminal","active":false,"dirty":true,"closable":false}],
       "newTabActions":[{"id":"term","title":"Terminal","icon":"TerminalWindow"}]}
      """)
    XCTAssertEqual(state.tabs.map(\.id), ["a", "b"])
    XCTAssertEqual(state.tabs[0].path, "src/main.rs")
    XCTAssertNil(state.tabs[1].path)
    XCTAssertTrue(state.tabs[1].dirty)
    XCTAssertEqual(state.newTabActions.first?.icon, "TerminalWindow")
  }

  func testTabsStateRejectsMissingField() {
    XCTAssertThrowsError(try decode(TabsState.self, #"{"tabs":[]}"#))
  }

  func testTabIcons() {
    XCTAssertEqual(TabIcon.of(kind: "terminal", path: nil), .terminalWindow)
    XCTAssertEqual(TabIcon.of(kind: "file", path: "a/b.SWIFT"), .fileCode)
    XCTAssertEqual(TabIcon.of(kind: "file", path: "notes.md"), .fileText)
    XCTAssertEqual(TabIcon.of(kind: "file", path: "x.unknown"), .file)
    XCTAssertEqual(TabIcon.of(kind: "file", path: nil), .file)
  }

  func testHostPickerState() throws {
    let state = try decode(
      HostPickerState.self,
      """
      {"open":true,"hosts":[{"id":"h1","name":"lab","endpoint":"https://lab:7777","state":"up"}],
       "revoking":["h1"],"shownHost":"h1"}
      """)
    XCTAssertTrue(state.open)
    XCTAssertEqual(state.list.hosts.map(\.name), ["lab"])
    XCTAssertEqual(state.list.revoking, ["h1"])
    XCTAssertEqual(state.list.shownHost, "h1")
  }

  func testHostListStateIsTheSettingsHostsSnapshot() throws {
    let list = try decode(
      HostListState.self,
      #"{"hosts":[{"id":"h1","name":"lab","endpoint":"e","state":"down"}],"revoking":[],"shownHost":null}"#)
    XCTAssertNil(list.shownHost)
    XCTAssertEqual(list.hosts.first?.state, "down")
  }

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
       "reveal":"chatList","ack":4,"settings":{"open":true,"theme":"crowbar","themes":[{"id":"crowbar","name":"Crowbar"}],"pages":[{"id":"hosts","label":"Hosts","icon":"HardDrives"}]},"appearance":"dark","hostPlace":"local",
       "palette":{"surface":"#000","raised":"#111","text":"#fff","secondaryText":"#888","brandLight":"#606c38","brandDark":"#98a86a"}}
      """)
    XCTAssertEqual(state.connect?.stage, .fingerprint)
    XCTAssertNil(state.connect?.error)
    XCTAssertEqual(state.headers?.chat, [])
    XCTAssertEqual(state.headers?.ide.first?.intent, nil)
    XCTAssertEqual(state.ack, 4)
    XCTAssertEqual(state.reveal, .chatList)
    XCTAssertEqual(Reveal.none.screen, .chat)
    XCTAssertEqual(Reveal.files.screen, .files)
    XCTAssertEqual(state.hostPlace, "local")
    XCTAssertEqual(state.settings?.theme, "crowbar")
    XCTAssertEqual(state.settings?.themes, [ThemeChoice(id: "crowbar", name: "Crowbar")])
    XCTAssertEqual(state.settings?.pages.first?.icon, "HardDrives")
    XCTAssertNotNil(state.palette.flatMap(ChromePalette.init)?.brandLight)
    XCTAssertNotNil(state.palette.flatMap(ChromePalette.init)?.brandDark)
  }

  func testPaletteWithUnparsableColourIsNil() throws {
    let state = try decode(
      PaletteState.self,
      ##"{"surface":"#000","raised":"nope","text":"#fff","secondaryText":"#888"}"##)
    XCTAssertNil(ChromePalette(state))
  }

  func testPaletteCarriesTheSidebarPaletteWhenTheThemeHasOne() throws {
    let state = try decode(
      PaletteState.self,
      ##"""
      {"surface":"#fcfcfa","raised":"#f1f0ea","text":"#1b1b19","secondaryText":"#666",
       "sidebar":{"text":"oklch(0.94 0.008 106.7)","brand":"#98a86a","brandText":"#1b1b19"}}
      """##)
    XCTAssertNotNil(ChromePalette(state)?.sidebar)
  }

  func testPaletteWithoutSidebarPaletteKeepsThePlainChrome() throws {
    let plain = try decode(
      PaletteState.self,
      ##"{"surface":"#fff","raised":"#eee","text":"#000","secondaryText":"#666","sidebar":null}"##)
    XCTAssertNotNil(ChromePalette(plain))
    XCTAssertNil(ChromePalette(plain)?.sidebar)
    let unparsable = try decode(
      PaletteState.self,
      ##"{"surface":"#fff","raised":"#eee","text":"#000","secondaryText":"#666","sidebar":{"text":"nope","brand":"#98a86a","brandText":"#000"}}"##)
    XCTAssertNil(ChromePalette(unparsable)?.sidebar)
  }

  func testUnknownRevealAndStageAreRejected() {
    XCTAssertThrowsError(try decode(Reveal.self, #""sideways""#))
    XCTAssertThrowsError(try decode(ConnectStage.self, #""done""#))
  }

  func testModelPickerState() throws {
    let state = try decode(
      ModelPickerState.self,
      """
      {"open":true,"providers":[{"id":"claude","name":"Claude","fallbackIcon":"Robot",
        "models":[{"id":"m1","name":"Model 1"}]}],
       "selected":{"providerId":"claude","modelId":"m1"},"efforts":["low","high"],"effort":null}
      """)
    XCTAssertNil(state.providers[0].icon)
    XCTAssertNil(state.providers[0].models[0].detail)
    XCTAssertEqual(state.selected, PickerSelection(providerId: "claude", modelId: "m1"))
    XCTAssertEqual(state.efforts, ["low", "high"])
  }

  func testContextStateHidesMeterWithoutPercent() throws {
    let state = try decode(
      ContextState.self, #"{"usedLabel":"1k","canCompact":false,"compacting":false}"#)
    XCTAssertNil(state.percent)
  }

  func testDialogOptions() throws {
    let prompt = try decode(
      Dialogs.PromptOptions.self,
      #"{"title":"t","value":"v","placeholder":"p","confirmLabel":"OK"}"#)
    XCTAssertNil(prompt.message)
    XCTAssertThrowsError(try decode(Dialogs.ConfirmOptions.self, #"{"title":"t"}"#))
  }

  func testIntentPayloadsCarryTheirType() {
    XCTAssertEqual(ChromeIntent.tap("files").payload["type"] as? String, "tap")
    XCTAssertEqual(ChromeIntent.tap("files").payload["id"] as? String, "files")
    let address = ChromeIntent.submitAddress(name: "n", address: "a").payload
    XCTAssertEqual(address["type"] as? String, "submitAddress")
    XCTAssertEqual(address["address"] as? String, "a")
  }

  @available(iOS 26.0, *)
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
