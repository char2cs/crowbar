import UIKit
import WebKit
import XCTest

@testable import tauri_plugin_crowbar_chrome

final class KeyboardFitTests: XCTestCase {
  private let screen = CGRect(x: 0, y: 0, width: 400, height: 800)
  private let keyboard = CGRect(x: 0, y: 500, width: 400, height: 300)

  private func post(_ name: Notification.Name) {
    NotificationCenter.default.post(
      name: name,
      object: nil,
      userInfo: [
        UIResponder.keyboardFrameEndUserInfoKey: NSValue(cgRect: keyboard),
        UIResponder.keyboardAnimationDurationUserInfoKey: 0.0,
        UIResponder.keyboardAnimationCurveUserInfoKey: UInt(7),
      ]
    )
  }

  func testWebviewSitsAboveTheKeyboardAndFillsTheHostOnceItHides() {
    let host = UIView(frame: screen)
    let webview = WKWebView(frame: screen)
    host.addSubview(webview)
    let fit = KeyboardFit(webview: webview)
    var fitted = 0
    fit.fitted = { fitted += 1 }

    post(UIResponder.keyboardWillChangeFrameNotification)
    XCTAssertEqual(webview.bounds.height, 500)
    XCTAssertEqual(webview.center.y, 250)

    post(UIResponder.keyboardWillHideNotification)
    XCTAssertEqual(webview.bounds.height, 800)
    XCTAssertEqual(fitted, 2)
    withExtendedLifetime(fit) {}
  }
}
