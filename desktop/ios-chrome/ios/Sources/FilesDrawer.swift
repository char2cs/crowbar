import UIKit
import WebKit

/// The IDE's files, as the mirror of the chat list: the live IDE card slides left over the
/// files page, leaving a strip at the left edge. The web owns whether it is open; a right-edge
/// swipe on the IDE and the strip report `openFiles` and `closeFiles`.
@available(iOS 26.0, *)
final class FilesDrawer: NSObject {
  private weak var host: UIView?
  private weak var webview: WKWebView?
  private let layer: UIView
  private let send: (ChromeIntent) -> Void
  private let dim = UIView()
  private let shadow = UIView()
  private let catcher = UIView()
  private var swipe: DrawerSwipe?

  /// The IDE is on screen, so the drawer may open.
  var enabled = false
  /// The IDE's native header, which slides with the IDE card.
  var riders: [UIView] = []
  private var open = false
  private var dragging = false
  private var progress: CGFloat = 0

  init(host: UIView, webview: WKWebView, layer: UIView, send: @escaping (ChromeIntent) -> Void) {
    self.host = host
    self.webview = webview
    self.layer = layer
    self.send = send
    super.init()
    layer.frame = host.bounds
    layer.autoresizingMask = [.flexibleWidth, .flexibleHeight]
    layer.isHidden = true
    host.insertSubview(layer, belowSubview: webview)

    dim.backgroundColor = .black
    dim.alpha = 0
    dim.isUserInteractionEnabled = false
    dim.frame = host.bounds
    dim.autoresizingMask = [.flexibleWidth, .flexibleHeight]
    dim.isHidden = true
    host.insertSubview(dim, aboveSubview: layer)

    shadow.backgroundColor = .systemBackground
    shadow.layer.shadowColor = UIColor.black.cgColor
    shadow.layer.shadowOpacity = 0.18
    shadow.layer.shadowRadius = 24
    shadow.layer.cornerCurve = .continuous
    shadow.isUserInteractionEnabled = false
    shadow.isHidden = true
    host.insertSubview(shadow, aboveSubview: dim)

    catcher.isHidden = true
    catcher.isAccessibilityElement = true
    catcher.accessibilityTraits = .button
    catcher.accessibilityLabel = "Close files"
    catcher.addGestureRecognizer(UITapGestureRecognizer(target: self, action: #selector(tapStrip)))
    catcher.addGestureRecognizer(UIPanGestureRecognizer(target: self, action: #selector(pan(_:))))
    host.addSubview(catcher)

    let swipe = DrawerSwipe(host: host, webview: webview, target: self, action: #selector(pan(_:)))
    // From the IDE's right edge only: its editor and terminal scroll sideways themselves.
    swipe.motion = { [weak self, weak host] start in
      guard let self, let host, enabled, !open else { return nil }
      return start.x > host.bounds.width - DrawerSwipe.edge ? .leftward : nil
    }
    self.swipe = swipe
  }

  /// The web's word on whether the files are out. A drag in progress reports its own result.
  func apply(open next: Bool) {
    guard !dragging, next != open || progress != (next ? 1 : 0) else { return }
    open = next
    animate(to: next ? 1 : 0, velocity: 0)
  }

  @objc private func tapStrip() {
    Haptics.tap()
    send(.of("closeFiles"))
  }

  @objc private func pan(_ gesture: UIPanGestureRecognizer) {
    guard let host else { return }
    let span = -(host.bounds.width - RevealController.strip)
    let dx = gesture.translation(in: host).x
    switch gesture.state {
    case .began:
      dragging = true
      layer.isHidden = false
    case .changed:
      layout(max(0, min(1, (open ? 1 : 0) + dx / span)))
    case .ended, .cancelled, .failed:
      dragging = false
      let fling = gesture.velocity(in: host).x / span
      let commit = gesture.state == .ended && (progress + fling * 0.15) > 0.5
      if commit != open {
        Haptics.tap()
        send(.of(commit ? "openFiles" : "closeFiles"))
      }
      open = commit
      animate(to: commit ? 1 : 0, velocity: fling)
    default:
      break
    }
  }

  private func animate(to end: CGFloat, velocity: CGFloat) {
    UIView.animate(
      springDuration: 0.42, bounce: 0.08, initialSpringVelocity: abs(velocity),
      options: [.allowUserInteraction, .beginFromCurrentState]
    ) {
      self.layout(end)
    } completion: { _ in
      self.rest(end)
    }
  }

  // Layers only appear here (it runs inside animations); `rest` hides them once still.
  private func layout(_ amount: CGFloat) {
    guard let host, let webview else { return }
    progress = amount
    let width = host.bounds.width
    let offset = -amount * (width - RevealController.strip)
    webview.transform = CGAffineTransform(translationX: offset, y: 0)
    for rider in riders { rider.transform = webview.transform }
    let radius = amount > 0 ? 44 * min(1, amount * 4) : 0
    webview.layer.cornerRadius = radius
    webview.layer.cornerCurve = .continuous
    webview.clipsToBounds = amount > 0
    shadow.bounds = CGRect(origin: .zero, size: host.bounds.size)
    shadow.center = CGPoint(x: host.bounds.midX + offset, y: host.bounds.midY)
    shadow.layer.cornerRadius = radius
    shadow.layer.shadowPath = UIBezierPath(roundedRect: shadow.bounds, cornerRadius: radius).cgPath
    dim.alpha = 0.22 * (1 - amount)
    if amount > 0 {
      layer.isHidden = false
      dim.isHidden = false
      shadow.isHidden = false
    }
    catcher.isHidden = amount < 1
    catcher.frame = CGRect(x: 0, y: 0, width: RevealController.strip, height: host.bounds.height)
  }

  private func rest(_ amount: CGFloat) {
    guard progress == amount, !dragging else { return }
    layer.isHidden = amount == 0
    dim.isHidden = amount == 0
    shadow.isHidden = amount == 0
  }
}
