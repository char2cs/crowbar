import SwiftUI
import UIKit
import WebKit

@available(iOS 26.0, *)
extension ChromeOverlays {
  static let leadingSide = "leading"
  static let trailingSide = "trailing"

  func makeWelcome() -> UIView? {
    mount(WelcomeButton(model: model)) { view, host in
      let centerX = view.centerXAnchor.constraint(equalTo: host.centerXAnchor)
      let centerY = view.centerYAnchor.constraint(
        equalTo: host.centerYAnchor,
        constant: Metric.welcomeLift
      )
      return [centerX, centerY]
    }
  }

  func makeTabStrip() -> UIView? {
    let content = TabStrip(model: tabs, chrome: model)
    return mount(content, style: .fillHidden) { view, host in
      let inset = GlassCircle.edgeInset + GlassCircle.diameter + Spacing.small
      let top = view.topAnchor.constraint(equalTo: host.safeAreaLayoutGuide.topAnchor)
      let height = view.heightAnchor.constraint(equalToConstant: Metric.tapTarget)
      let leading = view.leadingAnchor.constraint(equalTo: host.leadingAnchor, constant: inset)
      let trailing = view.trailingAnchor.constraint(equalTo: host.trailingAnchor, constant: -inset)
      return [top, height, leading, trailing]
    }
  }

  func corners(_ header: Header) -> [UIView] {
    [Self.leadingSide, Self.trailingSide].compactMap { corner(header, $0) }
  }

  private func corner(
    _ header: Header,
    _ side: String
  ) -> UIView? {
    let content = CornerButtons(model: model, header: header, side: side)
    return mount(content, style: .intrinsicShown) { view, host in
      let inset = GlassCircle.edgeInset
      let top = view.topAnchor.constraint(equalTo: host.safeAreaLayoutGuide.topAnchor)
      let edge =
        side == Self.leadingSide
        ? view.leadingAnchor.constraint(equalTo: host.leadingAnchor, constant: inset)
        : view.trailingAnchor.constraint(equalTo: host.trailingAnchor, constant: -inset)
      return [top, edge]
    }
  }

  func listLayer(_ page: WKWebView) -> UIView {
    let layer = pageLayer(page, trailing: NavigationController.strip)
    let foot = UIHostingController(rootView: ChatListFoot(model: model))
    foot.view.backgroundColor = .clear
    foot.sizingOptions = .intrinsicContentSize
    foot.view.translatesAutoresizingMaskIntoConstraints = false
    layer.addSubview(foot.view)
    foot.view.leadingAnchor.constraint(equalTo: page.leadingAnchor).isActive = true
    foot.view.trailingAnchor.constraint(equalTo: page.trailingAnchor).isActive = true
    foot.view.bottomAnchor.constraint(equalTo: layer.safeAreaLayoutGuide.bottomAnchor)
      .isActive = true
    hosted.append(foot)
    return layer
  }

  func paintSurface(
    under webview: WKWebView,
    host: UIView
  ) {
    surfaces.append(
      webview.observe(\.themeColor, options: [.initial, .new]) { [weak host] page, _ in
        let surface = page.themeColor ?? UIColor(named: "LaunchBackground") ?? .systemBackground
        host?.backgroundColor = surface
        host?.window?.backgroundColor = surface
        page.backgroundColor = surface
        page.scrollView.backgroundColor = surface
        page.underPageBackgroundColor = surface
      }
    )
  }

  func pageLayer(
    _ page: WKWebView,
    leading: CGFloat = 0,
    trailing: CGFloat = 0
  ) -> UIView {
    let layer = UIView()
    surfaces.append(
      page.observe(\.themeColor, options: [.initial, .new]) { [weak layer] page, _ in
        let ground = page.themeColor ?? .systemBackground
        layer?.backgroundColor = ground
        page.underPageBackgroundColor = ground
        page.scrollView.backgroundColor = ground
      }
    )
    page.translatesAutoresizingMaskIntoConstraints = false
    layer.addSubview(page)
    page.topAnchor.constraint(equalTo: layer.topAnchor).isActive = true
    page.bottomAnchor.constraint(equalTo: layer.bottomAnchor).isActive = true
    page.leadingAnchor.constraint(equalTo: layer.leadingAnchor, constant: leading)
      .isActive = true
    page.trailingAnchor.constraint(equalTo: layer.trailingAnchor, constant: -trailing)
      .isActive = true
    return layer
  }

  func fadeIn(_ view: UIView?) {
    guard let view else { return }
    view.isHidden = false
    UIView.animate(
      withDuration: Metric.fastDuration,
      animations: { view.alpha = 1 },
      completion: { _ in view.isHidden = view.alpha == 0 }
    )
  }

  func fadeOut(_ view: UIView?) {
    guard let view else { return }
    UIView.animate(
      withDuration: Metric.fastDuration,
      animations: { view.alpha = 0 },
      completion: { _ in view.isHidden = view.alpha == 0 }
    )
  }

  private func mount<Content: View>(
    _ content: Content,
    style: MountStyle = .intrinsicHidden,
    place: (UIView, UIView) -> [NSLayoutConstraint]
  ) -> UIView? {
    guard let host = webview?.superview ?? webview else { return nil }
    let controller = UIHostingController(rootView: content)
    controller.view.backgroundColor = .clear
    if style.sized { controller.sizingOptions = .intrinsicContentSize }
    controller.view.translatesAutoresizingMaskIntoConstraints = false
    controller.view.alpha = 0
    controller.view.isHidden = style.hidden
    host.addSubview(controller.view)
    NSLayoutConstraint.activate(place(controller.view, host))
    hosted.append(controller)
    return controller.view
  }
}
