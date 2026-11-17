import UIKit
import WebKit
import XCTest

@testable import tauri_plugin_crowbar_chrome

@available(iOS 26.0, *)
final class NavigationControllerTests: XCTestCase {
  private let screen = OverlaysRig.screen

  private func makeHost() -> (
    host: UIView,
    webview: WKWebView
  ) {
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
      layers: NavigationLayers(list: list, files: files),
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
      layers: NavigationLayers(list: UIView(), files: UIView()),
      send: { _ in }
    )
    let before = host.subviews.count
    navigation.willOpenIde()
    XCTAssertEqual(host.subviews.count, before + 1)
    navigation.willOpenIde()
    XCTAssertEqual(host.subviews.count, before + 1)
  }
}
