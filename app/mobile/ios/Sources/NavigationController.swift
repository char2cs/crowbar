import UIKit
import WebKit

@available(iOS 26.0, *)
final class NavigationController: NSObject {
  static let strip: CGFloat = 72

  private weak var host: UIView?
  private weak var webview: WKWebView?
  private let send: (ChromeIntent) -> Void
  private let list: UIView
  private let files: UIView
  private let dim = UIView()
  private let shadow = UIView()
  private let catcher = StripCatcher()
  private var snapshot: UIView?
  private var swipe: DrawerSwipe?
  private var link: CADisplayLink?
  private var lastTick: CFTimeInterval = 0
  private var machine: NavigationMachine

  var chatHeader: [UIView] = []
  var ideHeader: [UIView] = []
  var canOpenIde: () -> Bool = { false }
  var swipesEnabled: () -> Bool = { true }

  init(
    host: UIView,
    webview: WKWebView,
    list: UIView,
    files: UIView,
    send: @escaping (ChromeIntent) -> Void
  ) {
    self.host = host
    self.webview = webview
    self.list = list
    self.files = files
    self.send = send
    machine = NavigationMachine(width: Double(host.bounds.width), strip: Double(Self.strip))
    super.init()
    install(in: host, webview: webview)
    render()
  }

  deinit { link?.invalidate() }

  func web(reports reveal: Reveal, ack: Int) {
    run(machine.web(reports: reveal.screen, ack: ack))
    resume()
  }

  func willOpenIde() {
    run(machine.willOpenIde())
  }

  private func install(in host: UIView, webview: WKWebView) {
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
    shadow.layer.shadowRadius = 24
    shadow.layer.cornerCurve = .continuous
    shadow.isUserInteractionEnabled = false
    host.insertSubview(shadow, belowSubview: webview)

    webview.layer.cornerCurve = .continuous
    webview.layer.zPosition = Z.web

    catcher.isHidden = true
    catcher.isAccessibilityElement = true
    catcher.accessibilityTraits = .button
    catcher.close = { [weak self] in self?.tapStrip() }
    catcher.layer.zPosition = Z.catcher
    catcher.addGestureRecognizer(UITapGestureRecognizer(target: self, action: #selector(tapStrip)))
    host.addSubview(catcher)

    let swipe = DrawerSwipe(host: host, webview: webview, target: self, action: #selector(pan(_:)))
    swipe.claim = { [weak self] start in self?.claim(at: start) }
    self.swipe = swipe
  }

  private func claim(at start: CGPoint) -> DrawerSwipe.Claim? {
    guard let host, swipesEnabled() else { return nil }
    let width = host.bounds.width
    if machine.isMoving { return .init(motion: .either, overridesScrollers: true) }
    switch Int(machine.position.rounded()) {
    case 0 where start.x > width - Self.strip:
      return .init(motion: .leftward, overridesScrollers: true)
    case 1 where start.x < width * 0.4:
      return .init(motion: .rightward)
    case 1 where start.x > width * 0.6 && canOpenIde():
      return .init(motion: .leftward)
    // PLATFORM: The IDE's editor and terminal scroll sideways themselves: only its edges swipe.
    case 2 where start.x < DrawerSwipe.edge:
      return .init(motion: .rightward)
    case 2 where start.x > width - DrawerSwipe.edge:
      return .init(motion: .leftward)
    case 3 where start.x < Self.strip:
      return .init(motion: .rightward, overridesScrollers: true)
    default:
      return nil
    }
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
    link.preferredFrameRateRange = CAFrameRateRange(minimum: 60, maximum: 120, preferred: 120)
    link.add(to: .main, forMode: .common)
    self.link = link
    render()
  }

  func step(_ link: CADisplayLink) {
    let dt = lastTick == 0 ? link.duration : link.timestamp - lastTick
    lastTick = link.timestamp
    run(machine.tick(min(dt, 1.0 / 30)))
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

  private func freezeChat() {
    guard snapshot == nil, let webview, let host else { return }
    let frozen: UIView
    if let taken = webview.snapshotView(afterScreenUpdates: false) {
      frozen = taken
    } else {
      frozen = UIView()
      frozen.backgroundColor = webview.backgroundColor
    }
    frozen.bounds = webview.bounds
    frozen.center = webview.center
    frozen.layer.cornerCurve = .continuous
    frozen.clipsToBounds = true
    frozen.isUserInteractionEnabled = false
    frozen.layer.zPosition = Z.snapshot
    host.addSubview(frozen)
    snapshot = frozen
  }

  static let welcomeZ = Z.welcome

  private enum Z {
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

  private func transform(_ x: Double) -> CGAffineTransform {
    CGAffineTransform(translationX: CGFloat(x), y: 0)
  }

  private func render() {
    guard let host, let webview else { return }
    machine.width = Double(host.bounds.width)
    let frames = machine.frames
    let width = host.bounds.width

    let webIsIde = machine.hasSnapshot || machine.web >= .ide
    webview.transform = transform(webIsIde ? frames.ideOffset : frames.chatOffset)
    snapshot?.transform = transform(frames.chatOffset)

    let moving: UIView? =
      frames.gap == nil ? nil : (frames.gap == 2 ? webview : (snapshot ?? webview))
    let radius = CGFloat(44 * min(1, frames.amount * 4))
    for card in [webview, snapshot].compactMap({ $0 }) {
      let rounded = card === moving
      card.layer.cornerRadius = rounded ? radius : 0
      card.clipsToBounds = rounded || card === snapshot
    }

    let offset = CGFloat(frames.gap == 2 ? frames.ideOffset : frames.chatOffset)
    shadow.bounds = CGRect(origin: .zero, size: host.bounds.size)
    shadow.center = CGPoint(x: host.bounds.midX + offset, y: host.bounds.midY)
    shadow.layer.cornerRadius = radius
    let beneath =
      frames.gap == 1
      ? webview.backgroundColor : (machine.position < 1 ? list : files).backgroundColor
    shadow.backgroundColor = beneath ?? .clear
    shadow.layer.shadowPath =
      UIBezierPath(roundedRect: shadow.bounds, cornerRadius: radius).cgPath
    shadow.layer.shadowOpacity = Float(frames.shadow)
    shadow.isHidden = frames.gap == nil
    dim.alpha = CGFloat(frames.dim)
    let overIde = frames.gap == 1
    dim.layer.zPosition = overIde ? Z.scrimOverIde : Z.scrim
    shadow.layer.zPosition = overIde ? Z.shadowOverIde : Z.shadow

    park(list, by: machine.position < 1 ? 0 : width)
    park(files, by: machine.position > 2 ? 0 : width)
    list.layer.zPosition = Z.underlay
    files.layer.zPosition = Z.underlay

    place(chatHeader, offset: frames.chatOffset, alpha: frames.chatHeaderAlpha, z: Z.chatHeader)
    place(ideHeader, offset: frames.ideOffset, alpha: frames.ideHeaderAlpha, z: Z.ideHeader)
    placeCatcher(width: width, height: host.bounds.height)
  }

  private func park(_ layer: UIView, by offset: CGFloat) {
    layer.transform = CGAffineTransform(translationX: offset, y: 0)
  }

  private func place(_ views: [UIView], offset: Double, alpha: Double, z: CGFloat) {
    for view in views {
      view.transform = transform(offset)
      view.alpha = CGFloat(alpha)
      view.isUserInteractionEnabled = alpha >= 1
      view.layer.zPosition = z
    }
  }

  private func placeCatcher(width: CGFloat, height: CGFloat) {
    let at = machine.phase == .rest ? Int(machine.position.rounded()) : -1
    catcher.isHidden = at != 0 && at != 3
    switch at {
    case 0:
      catcher.frame = CGRect(x: width - Self.strip, y: 0, width: Self.strip, height: height)
      catcher.accessibilityLabel = "Close chat list"
    case 3:
      catcher.frame = CGRect(x: 0, y: 0, width: Self.strip, height: height)
      catcher.accessibilityLabel = "Close files"
    default:
      break
    }
    // PLATFORM: The status bar's glyphs are drawn for the drawer, so the card's strip under them takes the
    // PLATFORM: drawer's colour; the card's own white would swallow the clock where it straddles the edge.
    catcher.band.backgroundColor = (at == 3 ? files : list).backgroundColor
    catcher.band.frame = CGRect(
      x: 0,
      y: 0,
      width: Self.strip,
      height: host?.safeAreaInsets.top ?? 0
    )
  }
}
