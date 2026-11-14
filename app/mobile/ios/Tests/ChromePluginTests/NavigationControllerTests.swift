import UIKit
import WebKit
import XCTest

@testable import tauri_plugin_crowbar_chrome

@available(iOS 26.0, *)
final class NavigationControllerTests: XCTestCase {
  private let screen = CGRect(x: 0, y: 0, width: 400, height: 800)

  private func makeHost() -> (host: UIView, webview: WKWebView) {
    let host = UIView(frame: screen)
    let webview = WKWebView(frame: screen)
    host.addSubview(webview)
    return (host, webview)
  }

  func testDrawerLayersSitBeneathTheWebview() throws {
    let (host, webview) = makeHost()
    let list = UIView()
    let files = UIView()
    let navigation = NavigationController(
      host: host,
      webview: webview,
      list: list,
      files: files,
      send: { _ in }
    )
    let webIndex = try XCTUnwrap(host.subviews.firstIndex(of: webview))
    XCTAssertLessThan(try XCTUnwrap(host.subviews.firstIndex(of: list)), webIndex)
    XCTAssertLessThan(try XCTUnwrap(host.subviews.firstIndex(of: files)), webIndex)
    withExtendedLifetime(navigation) {}
  }

  func testOpeningTheIdeFreezesTheChatOnce() {
    let (host, webview) = makeHost()
    let navigation = NavigationController(
      host: host,
      webview: webview,
      list: UIView(),
      files: UIView(),
      send: { _ in }
    )
    let before = host.subviews.count
    navigation.willOpenIde()
    XCTAssertEqual(host.subviews.count, before + 1)
    navigation.willOpenIde()
    XCTAssertEqual(host.subviews.count, before + 1)
  }
}
