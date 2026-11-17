// swiftlint:disable file_name - the checker names this test file after the source file it covers

import UIKit
import XCTest

@testable import tauri_plugin_crowbar_chrome

@available(iOS 26.0, *)
final class ChromeOverlaysSheetsTests: XCTestCase {
  private struct PickCase {
    let pick: (ChromeOverlays) -> (String) -> Void
    let type: String
    let id: String
  }

  private let cases = [
    PickCase(pick: ChromeOverlays.pickPlusItem, type: "pickPlusItem", id: "camera"),
    PickCase(pick: ChromeOverlays.pickAttachSource, type: "pickAttachSource", id: "photos"),
  ]

  func testPickingFromASheetSendsTheIntentOnceItIsClosed() {
    for item in cases {
      let rig = OverlaysRig()
      item.pick(rig.overlays)(item.id)
      XCTAssertEqual(rig.log.types, [item.type])
      XCTAssertEqual(rig.log.sent.first?.payload["id"] as? String, item.id)
    }
  }

  func testThemeAndPickerStateReachTheirModels() throws {
    let rig = OverlaysRig()
    let theme = try decode(
      ChromeState.self,
      ##"{"accent":"#ff0000","brandGround":"#00ff00","appearance":"dark"}"##
    )
    rig.overlays.applyTheme(theme)
    XCTAssertNotNil(rig.model.accent)
    XCTAssertNotNil(rig.model.brandGround)

    let picker = try decode(
      ModelPickerState.self,
      #"{"open":true,"providers":[],"efforts":[]}"#
    )
    rig.overlays.applyModelPicker(picker)
    XCTAssertEqual(rig.overlays.picker.state, picker)
  }
}
