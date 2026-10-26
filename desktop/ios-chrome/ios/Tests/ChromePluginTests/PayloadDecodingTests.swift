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
       "newTabActions":[{"id":"term","title":"Terminal","symbol":"apple.terminal"}]}
      """)
    XCTAssertEqual(state.tabs.map(\.id), ["a", "b"])
    XCTAssertEqual(state.tabs[0].path, "src/main.rs")
    XCTAssertNil(state.tabs[1].path)
    XCTAssertTrue(state.tabs[1].dirty)
    XCTAssertEqual(state.newTabActions.first?.symbol, "apple.terminal")
  }

  func testTabsStateRejectsMissingField() {
    XCTAssertThrowsError(try decode(TabsState.self, #"{"tabs":[]}"#))
  }

  func testTabSymbols() {
    XCTAssertEqual(TabSymbol.of(kind: "terminal", path: nil), "apple.terminal")
    XCTAssertEqual(TabSymbol.of(kind: "file", path: "a/b.SWIFT"), "swift")
    XCTAssertEqual(TabSymbol.of(kind: "file", path: "notes.md"), "doc.plaintext")
    XCTAssertEqual(TabSymbol.of(kind: "file", path: "x.rs"), "chevron.left.forwardslash.chevron.right")
    XCTAssertEqual(TabSymbol.of(kind: "file", path: "x.unknown"), "doc.text")
    XCTAssertEqual(TabSymbol.of(kind: "file", path: nil), "doc.text")
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
       "headers":{"chat":[],"ide":[{"id":"files","symbol":"folder","side":"trailing",
         "label":"Files","enabled":true}]},
       "reveal":"chatList","ack":4,"settings":{"open":true},"appearance":"dark",
       "palette":{"surface":"#000","raised":"#111","text":"#fff","secondaryText":"#888"}}
      """)
    XCTAssertEqual(state.connect?.stage, .fingerprint)
    XCTAssertNil(state.connect?.error)
    XCTAssertEqual(state.headers?.chat, [])
    XCTAssertEqual(state.headers?.ide.first?.intent, nil)
    XCTAssertEqual(state.ack, 4)
    XCTAssertEqual(state.reveal, .chatList)
    XCTAssertEqual(Reveal.none.screen, .chat)
    XCTAssertEqual(Reveal.files.screen, .files)
    XCTAssertEqual(state.settings, SettingsState(open: true))
    XCTAssertNotNil(state.palette.flatMap(ChromePalette.init))
  }

  func testPaletteWithUnparsableColourIsNil() throws {
    let state = try decode(
      PaletteState.self,
      ##"{"surface":"#000","raised":"nope","text":"#fff","secondaryText":"#888"}"##)
    XCTAssertNil(ChromePalette(state))
  }

  func testUnknownRevealAndStageAreRejected() {
    XCTAssertThrowsError(try decode(Reveal.self, #""sideways""#))
    XCTAssertThrowsError(try decode(ConnectStage.self, #""done""#))
  }

  func testModelPickerState() throws {
    let state = try decode(
      ModelPickerState.self,
      """
      {"open":true,"providers":[{"id":"claude","name":"Claude","symbol":"sparkle",
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
      "id": "del", "title": "Delete", "symbol": "trash", "destructive": true,
      "confirm": ["message": "Sure?"],
    ])
    XCTAssertEqual(full?.id, "del")
    XCTAssertEqual(full?.destructive, true)
    XCTAssertEqual(full?.confirm?.title, "Delete")
    XCTAssertEqual(full?.confirm?.message, "Sure?")

    let plain = RowMenu.action(["id": "rename", "title": "Rename"])
    XCTAssertEqual(plain?.destructive, false)
    XCTAssertNil(plain?.confirm)

    XCTAssertNil(RowMenu.action(["id": "x"]))
    XCTAssertNil(RowMenu.action(["title": "x"]))
  }
}
