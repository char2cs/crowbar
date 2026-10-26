import UIKit
import WebKit

/// The one horizontal pan of the app's navigation. It moves cards from anywhere in a zone, not
/// just the screen edge, while the page keeps its own gestures: vertical scrolls, a touch held
/// to select text, and sideways scrollers (unless the touch starts at the very edge or its
/// claim overrides them).
final class DrawerSwipe: NSObject, UIGestureRecognizerDelegate {
  enum Motion { case rightward, leftward, either }

  /// What a touch may do, decided where it began.
  struct Claim {
    var motion: Motion
    /// A card's own strip, or a card already moving: no sideways scroller under it matters.
    var overridesScrollers = false
  }

  /// Within this of the screen edge a swipe wins even over a sideways scroller.
  static let edge: CGFloat = 32
  /// Held this long before moving, a touch is the page's (text selection, a row's lift).
  private static let hold: TimeInterval = 0.35

  let pan = UIPanGestureRecognizer()
  /// The claim a touch starting at this point (in the host) may make; nil leaves it alone.
  var claim: (CGPoint) -> Claim? = { _ in nil }
  private var expected: Claim?
  private weak var webview: WKWebView?
  private var start: CGPoint = .zero
  private var touched: TimeInterval = 0

  init(host: UIView, webview: WKWebView, target: Any, action: Selector) {
    self.webview = webview
    super.init()
    pan.addTarget(target, action: action)
    pan.delegate = self
    host.addGestureRecognizer(pan)
  }

  func gestureRecognizer(_ gesture: UIGestureRecognizer, shouldReceive touch: UITouch) -> Bool {
    guard let host = pan.view else { return false }
    start = touch.location(in: host)
    touched = touch.timestamp
    expected = claim(start)
    return expected != nil
  }

  func gestureRecognizerShouldBegin(_ gesture: UIGestureRecognizer) -> Bool {
    guard let expected, let host = pan.view else { return false }
    let velocity = pan.velocity(in: host)
    guard abs(velocity.x) > abs(velocity.y) * 1.2 else { return false }
    let direction: Motion = velocity.x > 0 ? .rightward : .leftward
    guard expected.motion == .either || expected.motion == direction else { return false }
    // A card already moving is grabbed at once; a resting page waits to see it is not a hold.
    guard expected.overridesScrollers || ProcessInfo.processInfo.systemUptime - touched < Self.hold
    else { return false }
    let atEdge = start.x < Self.edge || start.x > host.bounds.width - Self.edge
    return atEdge || expected.overridesScrollers || !scrollsSideways(at: start, in: host)
  }

  // The page's pans wait for this one to fail, which a vertical or held touch does at once.
  func gestureRecognizer(
    _ gesture: UIGestureRecognizer, shouldBeRequiredToFailBy other: UIGestureRecognizer
  ) -> Bool {
    guard let webview, other is UIPanGestureRecognizer, let view = other.view else { return false }
    return view.isDescendant(of: webview)
  }

  /// A web scroller under the touch that can move sideways (code blocks, wide tables).
  private func scrollsSideways(at point: CGPoint, in host: UIView) -> Bool {
    guard let webview else { return false }
    var view = webview.hitTest(host.convert(point, to: webview), with: nil)
    while let current = view, current !== webview {
      if let scroller = current as? UIScrollView, scroller !== webview.scrollView,
        scroller.contentSize.width > scroller.bounds.width + 1
      {
        return true
      }
      view = current.superview
    }
    return false
  }
}
