// swiftlint:disable:this file_name - named after the source file it covers

import SwiftUI
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

  private static let tolerance: CGFloat = 0.01

  private func assertColor(
    _ color: Color?,
    red: CGFloat,
    green: CGFloat,
    blue: CGFloat
  ) throws {
    let resolved = UIColor(try XCTUnwrap(color))
    var seenRed: CGFloat = 0
    var seenGreen: CGFloat = 0
    var seenBlue: CGFloat = 0
    var seenAlpha: CGFloat = 0
    XCTAssertTrue(resolved.getRed(&seenRed, green: &seenGreen, blue: &seenBlue, alpha: &seenAlpha))
    XCTAssertEqual(seenRed, red, accuracy: Self.tolerance)
    XCTAssertEqual(seenGreen, green, accuracy: Self.tolerance)
    XCTAssertEqual(seenBlue, blue, accuracy: Self.tolerance)
    XCTAssertEqual(seenAlpha, 1, accuracy: Self.tolerance)
  }

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
    try assertColor(rig.model.accent, red: 1, green: 0, blue: 0)
    try assertColor(rig.model.brandGround, red: 0, green: 1, blue: 0)

    let picker = try decode(
      ModelPickerState.self,
      #"{"open":true,"providers":[],"efforts":[]}"#
    )
    rig.overlays.applyModelPicker(picker)
    XCTAssertEqual(rig.overlays.picker.state, picker)
  }
}
