import Tauri
import WebKit

@available(iOS 26.0, *)
enum SecondaryPage {
  static func make(
    like main: WKWebView,
    fragment: String,
    controller: WKUserContentController,
    scheme: AppScheme
  ) -> WKWebView {
    let config = WKWebViewConfiguration()
    config.websiteDataStore = main.configuration.websiteDataStore
    config.userContentController = controller
    // PLATFORM: Not wry's handler: it assumes its own webview class and crashes on this one.
    config.setURLSchemeHandler(scheme, forURLScheme: "tauri")
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
