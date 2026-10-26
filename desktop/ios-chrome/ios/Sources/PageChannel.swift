import Tauri
import WebKit

/// Snapshot channels to pages of the app in the chrome's own webviews (the chat list, the
/// settings page), which have no IPC. The main webview pushes each feed's snapshot raw: its
/// shape is the web's alone. A page installs the feed's global and posts the feed's ready
/// message, and is handed the latest snapshot then. Anything else it posts to `handler` goes
/// to `intercept`, then, unless taken there, to the main webview as an intent.
@available(iOS 26.0, *)
final class PageChannel: NSObject, WKScriptMessageHandler {
  private let handler: String
  private let send: (ChromeIntent) -> Void
  /// Ready message → the global it feeds.
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

  /// A new page's content controller, wired to this channel.
  func controller() -> WKUserContentController {
    let controller = WKUserContentController()
    controller.add(WeakMessageHandler(self), name: handler)
    return controller
  }

  /// The feed whose pages call `global`.
  func render(_ snapshot: String, to global: String) {
    latest[global] = snapshot
    for page in pages[global]?.allObjects ?? [] { push(global, to: page) }
  }

  private func push(_ global: String, to page: WKWebView) {
    page.evaluateJavaScript("window.\(global)?.(\(latest[global] ?? "null"))")
  }

  func userContentController(
    _ controller: WKUserContentController, didReceive message: WKScriptMessage
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

/// WKUserContentController holds its handlers strongly; this breaks the page's retain cycle.
private final class WeakMessageHandler: NSObject, WKScriptMessageHandler {
  private weak var target: WKScriptMessageHandler?

  init(_ target: WKScriptMessageHandler) {
    self.target = target
  }

  func userContentController(
    _ controller: WKUserContentController, didReceive message: WKScriptMessage
  ) {
    target?.userContentController(controller, didReceive: message)
  }
}

/// A second page of the app, beside the main one: same storage and app URL, at another route.
/// It has no Tauri IPC, so the route it loads must render from what it is handed.
@available(iOS 26.0, *)
enum SecondaryPage {
  static func make(
    like main: WKWebView, fragment: String, controller: WKUserContentController
  ) -> WKWebView {
    let config = WKWebViewConfiguration()
    // Same storage as the main page: theme and settings come from its localStorage.
    config.websiteDataStore = main.configuration.websiteDataStore
    config.userContentController = controller
    // Not wry's handler: it assumes its own webview class and crashes on this one.
    config.setURLSchemeHandler(AppScheme.shared, forURLScheme: "tauri")
    let page = WKWebView(frame: .zero, configuration: config)
    FormAccessory.hide(in: page)
    page.isOpaque = false
    page.backgroundColor = .clear
    page.scrollView.contentInsetAdjustmentBehavior = .never
    if let url = main.url, var parts = URLComponents(url: url, resolvingAgainstBaseURL: false) {
      parts.fragment = fragment
      if let address = parts.url { page.load(URLRequest(url: address)) }
    }
    return page
  }
}
