import Tauri
import WebKit

@available(iOS 26.0, *)
final class PageChannel: NSObject, WKScriptMessageHandler {
  private let handler: String
  private let send: (ChromeIntent) -> Void
  private let feeds: [String: String]
  private var latest: [String: String] = [:]
  private var pages: [String: NSHashTable<WKWebView>] = [:]
  var intercept: (_ type: String, _ body: [String: Any], _ page: WKWebView) -> Bool = { _, _, _ in
    false
  }

  init(handler: String, feeds: [String: String], send: @escaping (ChromeIntent) -> Void) {
    self.handler = handler
    self.feeds = feeds
    self.send = send
  }

  func controller() -> WKUserContentController {
    let controller = WKUserContentController()
    controller.add(WeakMessageHandler(self), name: handler)
    return controller
  }

  func render(_ snapshot: String, to global: String) {
    latest[global] = snapshot
    for page in pages[global]?.allObjects ?? [] { push(global, to: page) }
  }

  private func push(_ global: String, to page: WKWebView) {
    page.evaluateJavaScript("window.\(global)?.(\(latest[global] ?? "null"))")
  }

  func userContentController(
    _ controller: WKUserContentController,
    didReceive message: WKScriptMessage
  ) {
    guard let body = message.body as? [String: Any], let type = body["type"] as? String,
      let page = message.webView
    else { return }
    if let global = feeds[type] {
      let ready = pages[global] ?? NSHashTable.weakObjects()
      ready.add(page)
      pages[global] = ready
      push(global, to: page)
      return
    }
    if intercept(type, body, page) { return }
    guard let payload = JSTypes.coerceDictionaryToJSObject(body) else { return }
    send(ChromeIntent(payload: payload))
  }
}
