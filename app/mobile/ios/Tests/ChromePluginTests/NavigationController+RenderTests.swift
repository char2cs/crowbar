// swiftlint:disable:this file_name - named after the source file it covers

import UIKit
import WebKit
import XCTest

@testable import tauri_plugin_crowbar_chrome

@available(iOS 26.0, *)
final class NavigationControllerRenderTests: XCTestCase {
  private let screen = CGRect(x: 0, y: 0, width: 400, height: 800)

  private struct Rig {
    let navigation: NavigationController
    let host: UIView
    let webview: WKWebView
  }

  private func makeNavigation() -> Rig {
    let host = UIView(frame: screen)
    let webview = WKWebView(frame: screen)
    host.addSubview(webview)
    let navigation = NavigationController(
      host: host,
      webview: webview,
      layers: NavigationLayers(list: UIView(), files: UIView()),
      send: { _ in }
    )
    return Rig(navigation: navigation, host: host, webview: webview)
  }

  func testChatAtRestParksTheDrawersAndHidesTheShadowAndCatcher() {
    let rig = makeNavigation()
    let (navigation, host, webview) = (rig.navigation, rig.host, rig.webview)
    XCTAssertEqual(navigation.list.transform.tx, host.bounds.width)
    XCTAssertEqual(navigation.files.transform.tx, host.bounds.width)
    XCTAssertEqual(navigation.list.layer.zPosition, NavigationController.Depth.underlay)
    XCTAssertEqual(navigation.files.layer.zPosition, NavigationController.Depth.underlay)
    XCTAssertEqual(webview.transform, .identity)
    XCTAssertTrue(navigation.shadow.isHidden)
    XCTAssertTrue(navigation.catcher.isHidden)
  }

  func testRenderingKeepsTheChatHeaderVisibleAndInteractive() {
    let rig = makeNavigation()
    let navigation = rig.navigation
    let header = UIView()
    withExtendedLifetime(rig) {
      navigation.chatHeader = [header]
      navigation.render()
    }
    XCTAssertEqual(header.alpha, 1)
    XCTAssertTrue(header.isUserInteractionEnabled)
    XCTAssertEqual(header.layer.zPosition, NavigationController.Depth.chatHeader)
  }
}
