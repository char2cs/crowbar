// swiftlint:disable file_name - the checker names this test file after the source file it covers

import UIKit
import XCTest

@testable import tauri_plugin_crowbar_chrome

@available(iOS 26.0, *)
final class ChromeOverlaysJumpButtonTests: XCTestCase {
  private struct HideCase {
    let owner: String
    let alphaAfter: CGFloat
  }

  private let cases = [
    HideCase(owner: "chat", alphaAfter: 0),
    HideCase(owner: "ide", alphaAfter: 1),
  ]

  func testOnlyTheOwnerOfTheButtonCanHideIt() throws {
    for item in cases {
      let rig = OverlaysRig()
      rig.overlays.applyJumpToLatest(
        JumpToLatestState(owner: "chat", visible: true, bottom: 40, label: "Jump")
      )
      rig.overlays.applyJumpToLatest(
        JumpToLatestState(owner: item.owner, visible: false, bottom: 40, label: "Jump")
      )
      let view = try XCTUnwrap(rig.overlays.jumpButton)
      XCTAssertEqual(view.alpha, item.alphaAfter, item.owner)
    }
  }

  func testShowingTheButtonRevealsItAboveTheBottomGap() throws {
    let rig = OverlaysRig()
    let view = try XCTUnwrap(rig.overlays.jumpButton)
    XCTAssertTrue(view.isHidden)
    XCTAssertTrue(view.superview === rig.webview)
    rig.overlays.applyJumpToLatest(
      JumpToLatestState(owner: "chat", visible: true, bottom: 100, label: "Jump")
    )
    XCTAssertEqual(rig.model.jumpOwner, "chat")
    XCTAssertEqual(rig.model.jumpLabel, "Jump")
    XCTAssertFalse(view.isHidden)
    XCTAssertEqual(view.alpha, 1)
    XCTAssertEqual(view.center.x, rig.webview.bounds.midX)
    XCTAssertEqual(view.center.y, 800 - 100 - GlassCircle.diameter / 2)
  }
}
