import UIKit
import WebKit

@testable import tauri_plugin_crowbar_chrome

@available(iOS 26.0, *)
final class OverlaysRig {
  static let width: CGFloat = 400
  static let height: CGFloat = 800
  static let screen = CGRect(x: 0, y: 0, width: width, height: height)
  static let leadingInset: CGFloat = 72
  static let trailingInset: CGFloat = 8
  private static let hostStubBytes = 1

  let webview: WKWebView
  let model = ChromeModel()
  let log = IntentLog()
  let overlays: ChromeOverlays
  private let hostStub = UnsafeMutableRawPointer.allocate(
    byteCount: OverlaysRig.hostStubBytes,
    alignment: OverlaysRig.hostStubBytes
  )

  init(frame: CGRect = OverlaysRig.screen) {
    let log = log
    webview = WKWebView(frame: frame)
    overlays = ChromeOverlays(
      model: model,
      webview: webview,
      scheme: AppScheme(frontend: UnsafeRawPointer(hostStub)),
      send: { log.record($0) }
    )
  }

  deinit { hostStub.deallocate() }
}
