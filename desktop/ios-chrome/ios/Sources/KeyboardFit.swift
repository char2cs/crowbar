import UIKit
import WebKit

/// Shrinks the webview above the keyboard, with the keyboard's own animation. Otherwise WebKit
/// pans the whole page up to reveal a focused field before the web's layout can react.
/// A downward drag on the page puts the keyboard away, as a native transcript's does.
final class KeyboardFit: NSObject, UIGestureRecognizerDelegate {
  private weak var webview: WKWebView?
  private var up = false

  init(webview: WKWebView) {
    self.webview = webview
    super.init()
    webview.scrollView.contentInsetAdjustmentBehavior = .never
    webview.scrollView.isScrollEnabled = false
    // The transcript scrolls in WebKit's own inner scrollers, so `keyboardDismissMode` has no
    // scroller to go on: this watches drags alongside them instead, never claiming the touch.
    let drag = UIPanGestureRecognizer(target: self, action: #selector(dragged(_:)))
    drag.cancelsTouchesInView = false
    drag.delegate = self
    webview.addGestureRecognizer(drag)
    let center = NotificationCenter.default
    center.addObserver(
      self, selector: #selector(change(_:)),
      name: UIResponder.keyboardWillChangeFrameNotification, object: nil)
    center.addObserver(
      self, selector: #selector(change(_:)), name: UIResponder.keyboardWillHideNotification,
      object: nil)
  }

  @objc private func dragged(_ drag: UIPanGestureRecognizer) {
    guard up, drag.state == .began, let webview else { return }
    let velocity = drag.velocity(in: webview)
    if velocity.y > abs(velocity.x) { webview.endEditing(true) }
  }

  func gestureRecognizer(
    _ gesture: UIGestureRecognizer,
    shouldRecognizeSimultaneouslyWith other: UIGestureRecognizer
  ) -> Bool { true }

  @objc private func change(_ note: Notification) {
    guard let webview, let host = webview.superview, let info = note.userInfo,
      let end = (info[UIResponder.keyboardFrameEndUserInfoKey] as? NSValue)?.cgRectValue
    else { return }
    let hiding = note.name == UIResponder.keyboardWillHideNotification
    let keyboard = host.convert(end, from: nil)
    let overlap = hiding ? 0 : max(0, host.bounds.maxY - keyboard.minY)
    up = overlap > 0
    let duration = info[UIResponder.keyboardAnimationDurationUserInfoKey] as? Double ?? 0.25
    let curve = info[UIResponder.keyboardAnimationCurveUserInfoKey] as? UInt ?? 7
    UIView.animate(
      withDuration: duration, delay: 0, options: UIView.AnimationOptions(rawValue: curve << 16)
    ) {
      // Bounds and centre, not frame: the reveal may have the webview transformed.
      let height = host.bounds.height - overlap
      webview.bounds.size.height = height
      webview.center.y = height / 2
    }
  }
}
