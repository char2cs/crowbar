// swiftlint:disable:this file_name - the checker names this test file after the source file it covers

import UIKit
import WebKit
import XCTest

@testable import tauri_plugin_crowbar_chrome

@available(iOS 26.0, *)
final class ChromeOverlaysLayersTests: XCTestCase {
  func testLayersAreBuiltInsideTheirHosts() {
    let rig = OverlaysRig()
    let corners = rig.overlays.corners(.chat)
    XCTAssertEqual(corners.count, 2)
    XCTAssertTrue(corners.allSatisfy { $0.superview === rig.webview })
    XCTAssertEqual(rig.overlays.hosted.count, 2)

    let page = WKWebView()
    let layer = rig.overlays.pageLayer(page, leading: 72, trailing: 8)
    layer.frame = OverlaysRig.screen
    layer.layoutIfNeeded()
    XCTAssertTrue(page.superview === layer)
    XCTAssertEqual(page.frame, CGRect(x: 72, y: 0, width: 320, height: 800))
  }

  func testFadesEndAtFullOrZeroAlphaAndIgnoreNothing() {
    let rig = OverlaysRig()
    let view = UIView()
    view.alpha = 0
    view.isHidden = true
    rig.overlays.fadeIn(view)
    XCTAssertFalse(view.isHidden)
    XCTAssertEqual(view.alpha, 1)
    rig.overlays.fadeOut(view)
    XCTAssertEqual(view.alpha, 0)
    rig.overlays.fadeIn(nil)
    rig.overlays.fadeOut(nil)
  }
}
