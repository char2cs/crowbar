import UIKit
import WebKit

@available(iOS 26.0, *)
final class NavigationController: NSObject {
  static let strip: CGFloat = 72
  private static let leftZone: CGFloat = 0.4
  private static let rightZone: CGFloat = 0.6

  weak var host: UIView?
  weak var webview: WKWebView?
  private let send: (ChromeIntent) -> Void
  let list: UIView
  let files: UIView
  let dim = UIView()
  let shadow = UIView()
  let catcher = StripCatcher()
  var snapshot: UIView?
  private var swipe: DrawerSwipe?
  private var link: CADisplayLink?
  private var lastTick: CFTimeInterval = 0
  var machine: NavigationMachine

  enum Depth {
    static let underlay: CGFloat = 1
    static let scrim: CGFloat = 2
    static let shadow: CGFloat = 2.5
    static let web: CGFloat = 3
    static let ideHeader: CGFloat = 3.5
    static let welcome: CGFloat = 3.7
    static let scrimOverIde: CGFloat = 4
    static let shadowOverIde: CGFloat = 4.5
    static let snapshot: CGFloat = 5
    static let chatHeader: CGFloat = 6
    static let catcher: CGFloat = 10
  }

  struct HeaderPlacement {
    let offset: Double
    let alpha: Double
    let depth: CGFloat
  }

  var chatHeader: [UIView] = []
  var ideHeader: [UIView] = []
  var canOpenIde: () -> Bool = { false }
  var swipesEnabled: () -> Bool = { true }

  init(
    host: UIView,
    webview: WKWebView,
    layers: NavigationLayers,
    send: @escaping (ChromeIntent) -> Void
  ) {
    self.host = host
    self.webview = webview
    list = layers.list
    files = layers.files
    self.send = send
    machine = NavigationMachine(width: Double(host.bounds.width), strip: Double(Self.strip))
    super.init()
    install(in: host, webview: webview)
    render()
  }

  deinit { link?.invalidate() }

  func web(
    reports reveal: Reveal,
    ack: Int
  ) {
    run(machine.web(reports: reveal.screen, ack: ack))
    resume()
  }

  func willOpenIde() {
    run(machine.willOpenIde())
  }

  private func install(
    in host: UIView,
    webview: WKWebView
  ) {
    for layer in [list, files] {
      layer.frame = host.bounds
      layer.autoresizingMask = [.flexibleWidth, .flexibleHeight]
      host.insertSubview(layer, belowSubview: webview)
    }
    dim.backgroundColor = .black
    dim.alpha = 0
    dim.isUserInteractionEnabled = false
    dim.frame = host.bounds
    dim.autoresizingMask = [.flexibleWidth, .flexibleHeight]
    host.insertSubview(dim, belowSubview: webview)

    shadow.layer.shadowColor = UIColor.black.cgColor
    shadow.layer.shadowRadius = Metric.cardShadowRadius
    shadow.layer.cornerCurve = .continuous
    shadow.isUserInteractionEnabled = false
    host.insertSubview(shadow, belowSubview: webview)

    webview.layer.cornerCurve = .continuous
    webview.layer.zPosition = Depth.web

    catcher.isHidden = true
    catcher.isAccessibilityElement = true
    catcher.accessibilityTraits = .button
    catcher.close = { [weak self] in self?.tapStrip() }
    catcher.layer.zPosition = Depth.catcher
    catcher.addGestureRecognizer(UITapGestureRecognizer(target: self, action: #selector(tapStrip)))
    host.addSubview(catcher)

    let swipe = DrawerSwipe(host: host, webview: webview, target: self, action: #selector(pan(_:)))
    swipe.claim = { [weak self] start in self?.claim(at: start) }
    self.swipe = swipe
  }

  private func claim(at start: CGPoint) -> DrawerSwipe.Claim? {
    guard let host, swipesEnabled() else { return nil }
    if machine.isMoving { return .init(motion: .either, overridesScrollers: true) }
    let width = host.bounds.width
    switch Screen(rawValue: Int(machine.position.rounded())) {
    case .chatList: return claimChatList(at: start, width: width)
    case .chat: return claimChat(at: start, width: width)
    case .ide: return claimIde(at: start, width: width)
    case .files: return claimFiles(at: start)
    default: return nil
    }
  }

  private func claimChatList(
    at start: CGPoint,
    width: CGFloat
  ) -> DrawerSwipe.Claim? {
    guard start.x > width - Self.strip else { return nil }
    return .init(motion: .leftward, overridesScrollers: true)
  }

  private func claimChat(
    at start: CGPoint,
    width: CGFloat
  ) -> DrawerSwipe.Claim? {
    if start.x < width * Self.leftZone { return .init(motion: .rightward) }
    guard start.x > width * Self.rightZone, canOpenIde() else { return nil }
    return .init(motion: .leftward)
  }

  private func claimIde(
    at start: CGPoint,
    width: CGFloat
  ) -> DrawerSwipe.Claim? {
    // PLATFORM: ios: The IDE's editor and terminal scroll sideways themselves:
    // PLATFORM: ios: only its edges swipe.
    if start.x < DrawerSwipe.edge { return .init(motion: .rightward) }
    guard start.x > width - DrawerSwipe.edge else { return nil }
    return .init(motion: .leftward)
  }

  private func claimFiles(at start: CGPoint) -> DrawerSwipe.Claim? {
    guard start.x < Self.strip else { return nil }
    return .init(motion: .rightward, overridesScrollers: true)
  }

  @objc private func tapStrip() {
    Haptics.tap()
    run(machine.closeDrawer())
    resume()
  }

  @objc private func pan(_ gesture: UIPanGestureRecognizer) {
    guard let host else { return }
    let travel = Double(gesture.translation(in: host).x)
    switch gesture.state {
    case .began:
      let velocity = gesture.velocity(in: host).x
      guard let began = machine.beginDrag(toward: velocity > 0 ? .rightward : .leftward) else {
        return
      }
      run(began)
      machine.drag(translation: travel)
    case .changed:
      machine.drag(translation: travel)
    case .ended, .cancelled, .failed:
      let commands = machine.endDrag(
        velocity: Double(gesture.velocity(in: host).x),
        ending: gesture.state == .ended ? .released : .cancelled
      )
      if !commands.isEmpty { Haptics.tap() }
      run(commands)
      resume()
    default:
      return
    }
    render()
  }

  private func resume() {
    guard machine.isMoving, machine.phase != .dragging, link == nil else {
      render()
      return
    }
    lastTick = 0
    let link = CADisplayLink(target: LinkTarget(self), selector: #selector(LinkTarget.step(_:)))
    link.preferredFrameRateRange = CAFrameRateRange(
      minimum: Motion.minFrameRate,
      maximum: Motion.maxFrameRate,
      preferred: Motion.maxFrameRate
    )
    link.add(to: .main, forMode: .common)
    self.link = link
    render()
  }

  func step(_ link: CADisplayLink) {
    let elapsed = lastTick == 0 ? link.duration : link.timestamp - lastTick
    lastTick = link.timestamp
    run(machine.tick(min(elapsed, Motion.maxFrameStep)))
    render()
    if case .settling = machine.phase { return }
    link.invalidate()
    self.link = nil
  }

  private func run(_ commands: [NavigationCommand]) {
    for command in commands {
      switch command {
      case .send(let name, let seq):
        send(.of(name, ["seq": seq]))
      case .freezeChat:
        freezeChat()
      case .dropSnapshot:
        snapshot?.removeFromSuperview()
        snapshot = nil
      }
    }
  }

  private func blank(like webview: WKWebView) -> UIView {
    let view = UIView()
    view.backgroundColor = webview.backgroundColor
    return view
  }

  private func freezeChat() {
    guard snapshot == nil, let webview, let host else { return }
    let frozen = webview.snapshotView(afterScreenUpdates: false) ?? blank(like: webview)
    frozen.bounds = webview.bounds
    frozen.center = webview.center
    frozen.layer.cornerCurve = .continuous
    frozen.clipsToBounds = true
    frozen.isUserInteractionEnabled = false
    frozen.layer.zPosition = Depth.snapshot
    host.addSubview(frozen)
    snapshot = frozen
  }
}
