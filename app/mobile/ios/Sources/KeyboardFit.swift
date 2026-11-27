import UIKit
import WebKit

final class KeyboardFit: NSObject, UIGestureRecognizerDelegate {
  private static let half: CGFloat = 0.5

  private weak var webview: WKWebView?
  private var isKeyboardUp = false
  var fitted: () -> Void = {}

  init(webview: WKWebView) {
    self.webview = webview
    super.init()
    webview.scrollView.contentInsetAdjustmentBehavior = .never
    webview.scrollView.isScrollEnabled = false
    // PLATFORM: ios: The transcript scrolls in WebKit's own inner scrollers, so
    // PLATFORM: ios: `keyboardDismissMode` has no scroller to go on: this watches drags
    // PLATFORM: ios: alongside them instead, never claiming the touch.
    let drag = UIPanGestureRecognizer(target: self, action: #selector(dragged(_:)))
    drag.cancelsTouchesInView = false
    drag.delegate = self
    webview.addGestureRecognizer(drag)
    let center = NotificationCenter.default
    center.addObserver(
      self,
      selector: #selector(change(_:)),
      name: UIResponder.keyboardWillChangeFrameNotification,
      object: nil
    )
    center.addObserver(
      self,
      selector: #selector(change(_:)),
      name: UIResponder.keyboardWillHideNotification,
      object: nil
    )
  }

  @objc private func dragged(_ drag: UIPanGestureRecognizer) {
    guard isKeyboardUp, drag.state == .began, let webview else { return }
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
    isKeyboardUp = overlap > 0
    let duration =
      info[UIResponder.keyboardAnimationDurationUserInfoKey] as? Double ?? Motion.keyboardDuration
    let curve = info[UIResponder.keyboardAnimationCurveUserInfoKey] as? UInt ?? Motion.keyboardCurve
    UIView.animate(
      withDuration: duration,
      delay: 0,
      options: UIView.AnimationOptions(rawValue: curve << Motion.curveShift)
    ) {
      // PLATFORM: ios: Bounds and centre, not frame: the reveal may have the webview transformed.
      let height = host.bounds.height - overlap
      webview.bounds.size.height = height
      webview.center.y = height * Self.half
      self.fitted()
    }
  }
}
