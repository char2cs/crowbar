import SwiftUI
import WebKit

/// Settings as a native sheet around one page: the web's own settings, which navigate
/// themselves. Native here is only the grabber, the close button and the sheet, which takes
/// the page's surface colour (its theme-color) so the two read as one.
@available(iOS 26.0, *)
struct SettingsSheet: View {
  @StateObject private var surface: PageSurface
  let close: () -> Void

  init(page: WKWebView, close: @escaping () -> Void) {
    _surface = StateObject(wrappedValue: PageSurface(page: page))
    self.close = close
  }

  var body: some View {
    VStack(spacing: 0) {
      HStack {
        GlassCircle(symbol: "xmark", label: "Close", action: close)
        Spacer()
      }
      .padding(.horizontal, GlassCircle.edgeInset)
      .padding(.top, 16)
      .padding(.bottom, 8)
      SettingsPage(page: surface.page)
        .ignoresSafeArea(edges: .bottom)
    }
    .background(surface.color.ignoresSafeArea())
  }
}

/// A page's theme-color, as it changes.
@available(iOS 26.0, *)
final class PageSurface: ObservableObject {
  let page: WKWebView
  @Published private(set) var color = Color(uiColor: .systemBackground)
  private var watch: NSKeyValueObservation?

  init(page: WKWebView) {
    self.page = page
    watch = page.observe(\.themeColor, options: [.initial, .new]) { [weak self] page, _ in
      self?.color = Color(uiColor: page.themeColor ?? .systemBackground)
    }
  }
}

private struct SettingsPage: UIViewRepresentable {
  let page: WKWebView

  func makeUIView(context: Context) -> WKWebView { page }
  func updateUIView(_ view: WKWebView, context: Context) {}
}
