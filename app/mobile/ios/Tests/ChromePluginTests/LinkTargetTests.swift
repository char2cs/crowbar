import UIKit
import WebKit
import XCTest

@testable import tauri_plugin_crowbar_chrome

@available(iOS 26.0, *)
final class LinkTargetTests: XCTestCase {
  func testTargetDoesNotKeepTheControllerAlive() {
    let screen = CGRect(x: 0, y: 0, width: 400, height: 800)
    let host = UIView(frame: screen)
    let webview = WKWebView(frame: screen)
    host.addSubview(webview)
    var controller: NavigationController? = NavigationController(
      host: host,
      webview: webview,
      layers: NavigationLayers(list: UIView(), files: UIView()),
      send: { _ in }
    )
    weak var released = controller
    let target = controller.map { LinkTarget($0) }
    controller = nil
    XCTAssertNil(released)
    withExtendedLifetime(target) {}
  }
}
