import SwiftUI
import WebKit

struct HostedPage: UIViewRepresentable {
  let page: WKWebView

  func makeUIView(context: Context) -> WKWebView { page }
  func updateUIView(_ view: WKWebView, context: Context) {}
}
