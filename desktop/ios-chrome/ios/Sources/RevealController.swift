import SwiftUI
import UIKit
import WebKit

/// Slides the chat aside as a front card. Leading: the live webview itself slides right over the
/// chat list. Trailing: a snapshot of the chat slides left off the live webview, which the
/// web has switched to the IDE. The web owns which layer is current; edge swipes are reported.
@available(iOS 26.0, *)
final class RevealController: NSObject {
  static let strip: CGFloat = 72

  private weak var host: UIView?
  private weak var webview: WKWebView?
  private let send: (ChromeIntent) -> Void
  /// The chat list layer under the card.
  private let list: UIView
  private let dim = UIView()
  private let shadow = UIView()
  private let catcher = StripCatcher()
  private var snapshot: UIView?
  private var swipe: DrawerSwipe?

  private var target: Reveal = .none
  private var shown: Reveal = .none
  private var dragging: Reveal?
  private var progress: CGFloat = 0
  /// Another drawer is out over the IDE: the IDE's own edge swipe waits.
  var locked = false
  /// The native header over the card, which slides with it over the chat list.
  var riders: [UIView] = []

  init(host: UIView, webview: WKWebView, list: UIView, send: @escaping (ChromeIntent) -> Void) {
    self.host = host
    self.webview = webview
    self.list = list
    self.send = send
    super.init()
    install(in: host, below: webview)
  }

  /// The web's current layer. A drag in progress finishes first and reports its own result.
  func apply(_ reveal: Reveal) {
    target = reveal
    guard dragging == nil else { return }
    settle(to: reveal, velocity: 0)
  }

  /// The chat, frozen, before the web swaps it for the IDE beneath.
  func freezeChat() {
    guard snapshot == nil, let webview, let host else { return }
    // The native header over the chat too: it leaves with the chat it belongs to.
    guard
      let frozen = host.resizableSnapshotView(
        from: webview.frame, afterScreenUpdates: false, withCapInsets: .zero)
    else { return }
    frozen.frame = webview.frame
    frozen.layer.cornerCurve = .continuous
    frozen.clipsToBounds = true
    // Over everything live, the IDE's own header included, which waits beneath it.
    host.addSubview(frozen)
    host.insertSubview(shadow, belowSubview: frozen)
    host.insertSubview(dim, belowSubview: shadow)
    snapshot = frozen
  }

  private var card: UIView? { snapshot ?? webview }

  private func install(in host: UIView, below webview: WKWebView) {
    list.frame = host.bounds
    list.autoresizingMask = [.flexibleWidth, .flexibleHeight]
    list.isHidden = true
    host.insertSubview(list, belowSubview: webview)

    dim.backgroundColor = .black
    dim.alpha = 0
    dim.isUserInteractionEnabled = false
    dim.frame = host.bounds
    dim.autoresizingMask = [.flexibleWidth, .flexibleHeight]
    host.insertSubview(dim, belowSubview: webview)

    shadow.backgroundColor = .systemBackground
    shadow.layer.shadowColor = UIColor.black.cgColor
    shadow.layer.shadowOpacity = 0
    shadow.layer.shadowRadius = 24
    shadow.layer.cornerCurve = .continuous
    shadow.isUserInteractionEnabled = false
    host.insertSubview(shadow, belowSubview: webview)

    // Over the visible strip of a slid-aside card: a tap or a drag back closes the reveal.
    catcher.isHidden = true
    // VoiceOver reaches the strip as a button, and the escape gesture closes the drawer too.
    catcher.isAccessibilityElement = true
    catcher.accessibilityTraits = .button
    catcher.accessibilityLabel = "Close chat list"
    catcher.close = { [weak self] in self?.tapStrip() }
    catcher.addGestureRecognizer(UITapGestureRecognizer(target: self, action: #selector(tapStrip)))
    catcher.addGestureRecognizer(UIPanGestureRecognizer(target: self, action: #selector(pan(_:))))
    host.addSubview(catcher)

    guard let webview = self.webview else { return }
    let swipe = DrawerSwipe(host: host, webview: webview, target: self, action: #selector(pan(_:)))
    // On the chat, the list from its left 40% and the IDE from its right 40%; on the IDE,
    // whose editor and terminal scroll sideways themselves, back from the left edge only.
    swipe.motion = { [weak self, weak host] start in
      guard let self, let host, !locked else { return nil }
      let width = host.bounds.width
      if shown == .none, start.x < width * 0.4 { return .rightward }
      if shown == .none, start.x > width * 0.6 { return .leftward }
      if shown == .ide, start.x < DrawerSwipe.edge { return .rightward }
      return nil
    }
    self.swipe = swipe
  }

  // MARK: gestures

  @objc private func tapStrip() {
    Haptics.tap()
    send(.of(shown == .ide ? "closeIde" : "closeChatList"))
  }

  @objc private func pan(_ gesture: UIPanGestureRecognizer) {
    guard let host else { return }
    let width = host.bounds.width
    let dx = gesture.translation(in: host).x
    let velocity = gesture.velocity(in: host).x
    switch gesture.state {
    case .began:
      dragging = dragKind(gesture)
      if dragging == .ide, shown == .none {
        freezeChat()
        send(.of("openIde"))
      }
      prepare(dragging ?? .none)
    case .changed:
      guard let kind = dragging else { return }
      let base: CGFloat = shown == kind ? 1 : 0
      let span = kind == .ide ? -width : width - Self.strip
      layout(kind, max(0, min(1, base + dx / span)))
    case .ended, .cancelled, .failed:
      guard let kind = dragging else { return }
      dragging = nil
      let span = kind == .ide ? -width : width - Self.strip
      let fling = velocity / span
      let open = gesture.state == .ended && (progress + fling * 0.15) > 0.5
      finish(kind, open: open, velocity: fling)
    default:
      break
    }
  }

  private func dragKind(_ gesture: UIPanGestureRecognizer) -> Reveal? {
    guard let swipe, gesture === swipe.pan else { return shown == .none ? nil : shown }
    if shown == .none { return swipe.expected == .rightward ? .chatList : .ide }
    return shown == .ide ? .ide : nil
  }

  private func finish(_ kind: Reveal, open: Bool, velocity: CGFloat) {
    let closed = kind == .chatList ? "closeChatList" : "closeIde"
    let was = shown
    if open != (was == kind) { Haptics.tap() }
    animate(kind, to: open ? 1 : 0, velocity: velocity) { [weak self] in
      guard let self else { return }
      shown = open ? kind : .none
      // The trailing reveal already said so when the drag began.
      if open, was != kind, kind == .chatList { send(.of("openChatList")) }
      if !open, was == kind || kind == .ide { send(.of(closed)) }
      // Swiped closed and the web already back on the chat: the frozen card can go.
      if shown == .none, target == .none { thaw() }
    }
  }

  // MARK: settling to the web's layer

  private func settle(to reveal: Reveal, velocity: CGFloat) {
    if reveal == shown {
      if reveal == .none { thaw() }
      return
    }
    if shown != .none {
      let leaving = shown
      animate(leaving, to: 0, velocity: velocity) { [weak self] in
        guard let self else { return }
        shown = .none
        thaw()
        if target != .none { settle(to: target, velocity: 0) }
      }
      return
    }
    if reveal == .ide { freezeChat() }
    prepare(reveal)
    animate(reveal, to: 1, velocity: velocity) { [weak self] in self?.shown = reveal }
  }

  private func thaw() {
    guard dragging == nil, shown == .none else { return }
    dropFrozen()
    layout(.none, 0)
    rest(.none, 0)
  }

  /// The frozen chat is only the IDE reveal's card; the live webview is everyone else's.
  private func dropFrozen() {
    snapshot?.removeFromSuperview()
    snapshot = nil
    if let webview, let host {
      host.insertSubview(shadow, belowSubview: webview)
      host.insertSubview(dim, belowSubview: shadow)
    }
  }

  // MARK: layout

  private func prepare(_ kind: Reveal) {
    list.isHidden = kind != .chatList
    // An IDE reveal cut short by the chat list leaves its frozen chat behind otherwise.
    if kind == .chatList { dropFrozen() }
  }

  private func animate(_ kind: Reveal, to end: CGFloat, velocity: CGFloat, done: @escaping () -> Void) {
    UIView.animate(
      springDuration: 0.42, bounce: 0.08, initialSpringVelocity: abs(velocity),
      options: [.allowUserInteraction, .beginFromCurrentState]
    ) {
      self.layout(kind, end)
    } completion: { _ in
      self.rest(kind, end)
      done()
    }
  }

  /// Hides what the card now fully covers, or what slid away, once nothing is moving.
  private func rest(_ kind: Reveal, _ amount: CGFloat) {
    // A newer animation or drag has moved on; its own end decides.
    guard progress == amount else { return }
    list.isHidden = !(kind == .chatList && amount > 0)
    // An IDE card slid fully away leaves no shadow along the screen's left edge.
    shadow.isHidden = amount == 0 || (kind == .ide && amount >= 1)
  }

  private func layout(_ kind: Reveal, _ amount: CGFloat) {
    guard let host, let card else { return }
    progress = amount
    let width = host.bounds.width
    let offset = kind == .ide ? -amount * width : amount * (width - Self.strip)
    let moved = CGAffineTransform(translationX: offset, y: 0)
    card.transform = moved
    for rider in riders { rider.transform = kind == .chatList ? moved : .identity }
    card.layer.cornerRadius = amount > 0 ? 44 * min(1, amount * 4) : 0
    card.clipsToBounds = amount > 0 || card === snapshot
    shadow.bounds = CGRect(origin: .zero, size: host.bounds.size)
    shadow.center = CGPoint(x: host.bounds.midX + offset, y: host.bounds.midY)
    shadow.layer.cornerRadius = card.layer.cornerRadius
    shadow.layer.shadowPath =
      UIBezierPath(roundedRect: shadow.bounds, cornerRadius: card.layer.cornerRadius).cgPath
    shadow.layer.shadowOpacity = amount > 0 ? 0.18 : 0
    // The layer underneath brightens as it is revealed.
    dim.alpha = kind == .none ? 0 : 0.22 * (1 - amount)
    // Layers only appear here: this runs inside animations, where hiding would land on the first
    // frame and blank the list while the card still slides over it. `rest` hides them.
    if amount > 0 {
      list.isHidden = kind != .chatList
      shadow.isHidden = false
    }
    catcher.isHidden = amount < 1
    if amount >= 1 {
      catcher.frame =
        kind == .ide
        ? CGRect(x: 0, y: 0, width: 0, height: 0)
        : CGRect(x: width - Self.strip, y: 0, width: Self.strip, height: host.bounds.height)
    }
  }
}

/// The visible strip of a slid-aside card, as one control that closes the drawer.
private final class StripCatcher: UIView {
  var close: () -> Void = {}

  override func accessibilityActivate() -> Bool {
    close()
    return true
  }

  override func accessibilityPerformEscape() -> Bool {
    close()
    return true
  }
}
