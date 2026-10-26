import UIKit
import WebKit

/// Presents the phone's screens: the one pan, the cards it moves, and each screen's header.
/// All state is the `NavigationMachine`'s; this renders its `frames` on every change and feeds
/// it the user's drags, the web's reports and the display link's ticks.
///
/// Layers, bottom to top: the chat list and the files page (parked off screen unless one is
/// being uncovered, so neither stops rendering), the scrim and shadow, the live webview, the IDE's
/// header, the frozen chat, the chat's header. The webview is the chat card until the web swaps
/// it for the IDE; the frozen chat is the chat card from then on.
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

  /// Each screen's header views: they move and fade with their own card.
  var chatHeader: [UIView] = []
  var ideHeader: [UIView] = []
  /// Whether the chat has an IDE to swipe to.
  var canOpenIde: () -> Bool = { false }

  init(
    host: UIView, webview: WKWebView, list: UIView, files: UIView,
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

  // MARK: from the web

  /// What the web shows now, and the newest of our intents it had applied when it said so.
  func web(reports reveal: Reveal, ack: Int) {
    run(machine.web(reports: reveal.screen, ack: ack))
    resume()
  }

  /// The web is about to put the IDE where the chat is.
  func willOpenIde() {
    run(machine.willOpenIde())
  }

  // MARK: installation

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

    shadow.backgroundColor = .systemBackground
    shadow.layer.shadowColor = UIColor.black.cgColor
    shadow.layer.shadowRadius = 24
    shadow.layer.cornerCurve = .continuous
    shadow.isUserInteractionEnabled = false
    host.insertSubview(shadow, belowSubview: webview)

    webview.layer.cornerCurve = .continuous
    webview.layer.zPosition = Z.web

    // Over the visible strip of a card slid aside: a tap puts it back.
    catcher.isHidden = true
    // VoiceOver reaches the strip as a button, and the escape gesture closes the drawer too.
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

  /// Where a touch may start a swipe, by the screen at rest. A card already moving is grabbed
  /// from anywhere, and either way.
  private func claim(at start: CGPoint) -> DrawerSwipe.Claim? {
    guard let host else { return nil }
    let width = host.bounds.width
    if machine.isMoving { return .init(motion: .either, overridesScrollers: true) }
    switch Int(machine.position.rounded()) {
    case 0 where start.x > width - Self.strip:
      return .init(motion: .leftward, overridesScrollers: true)
    case 1 where start.x < width * 0.4:
      return .init(motion: .rightward)
    case 1 where start.x > width * 0.6 && canOpenIde():
      return .init(motion: .leftward)
    // The IDE's editor and terminal scroll sideways themselves: only its edges swipe.
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

  // MARK: input

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
        velocity: Double(gesture.velocity(in: host).x), cancelled: gesture.state != .ended)
      if !commands.isEmpty { Haptics.tap() }
      run(commands)
      resume()
    default:
      return
    }
    render()
  }

  // MARK: motion

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

  fileprivate func step(_ link: CADisplayLink) {
    let dt = lastTick == 0 ? link.duration : link.timestamp - lastTick
    lastTick = link.timestamp
    run(machine.tick(min(dt, 1.0 / 30)))
    render()
    if case .settling = machine.phase { return }
    // A drag that took the card over has its own end; nothing else needs the link.
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

  /// The chat as it is now, before the web swaps it for the IDE beneath. Only the webview: the
  /// native headers are each their screen's and are never baked into a card.
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

  // MARK: layout

  private enum Z {
    static let underlay: CGFloat = 1
    static let scrim: CGFloat = 2
    static let shadow: CGFloat = 2.5
    static let web: CGFloat = 3
    static let ideHeader: CGFloat = 3.5
    static let scrimOverIde: CGFloat = 4
    static let shadowOverIde: CGFloat = 4.5
    static let snapshot: CGFloat = 5
    static let chatHeader: CGFloat = 6
    static let catcher: CGFloat = 10
  }

  private func transform(_ x: Double) -> CGAffineTransform {
    CGAffineTransform(translationX: CGFloat(x), y: 0)
  }

  /// Draws every layer from the machine's position: nothing here decides anything.
  private func render() {
    guard let host, let webview else { return }
    machine.width = Double(host.bounds.width)
    let frames = machine.frames
    let width = host.bounds.width

    // The webview is the IDE's card once the chat has its own (the frozen one).
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
    shadow.layer.shadowPath =
      UIBezierPath(roundedRect: shadow.bounds, cornerRadius: radius).cgPath
    shadow.layer.shadowOpacity = Float(frames.shadow)
    shadow.isHidden = frames.gap == nil
    dim.alpha = CGFloat(frames.dim)
    let overIde = frames.gap == 1
    dim.layer.zPosition = overIde ? Z.scrimOverIde : Z.scrim
    shadow.layer.zPosition = overIde ? Z.shadowOverIde : Z.shadow

    // Whatever is uncovered is the one page beneath; the other waits off screen, still drawn.
    park(list, except: machine.position < 1, by: width)
    park(files, except: machine.position > 2, by: width)
    list.layer.zPosition = Z.underlay
    files.layer.zPosition = Z.underlay

    place(chatHeader, offset: frames.chatOffset, alpha: frames.chatHeaderAlpha, z: Z.chatHeader)
    place(ideHeader, offset: frames.ideOffset, alpha: frames.ideHeaderAlpha, z: Z.ideHeader)
    placeCatcher(width: width, height: host.bounds.height)
  }

  private func park(_ layer: UIView, except visible: Bool, by width: CGFloat) {
    layer.transform = visible ? .identity : CGAffineTransform(translationX: width, y: 0)
  }

  private func place(_ views: [UIView], offset: Double, alpha: Double, z: CGFloat) {
    for view in views {
      view.transform = transform(offset)
      view.alpha = CGFloat(alpha)
      // A header on its way in or out is not a target yet.
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
  }
}

/// The display link retains its target; this one holds the controller weakly.
@available(iOS 26.0, *)
private final class LinkTarget: NSObject {
  private weak var controller: NavigationController?

  init(_ controller: NavigationController) {
    self.controller = controller
  }

  @objc func step(_ link: CADisplayLink) {
    guard let controller else { return link.invalidate() }
    controller.step(link)
  }
}

extension Reveal {
  var screen: Screen {
    switch self {
    case .none: return .chat
    case .chatList: return .chatList
    case .ide: return .ide
    case .files: return .files
    }
  }
}

/// The visible strip of a card slid aside, as one control that closes the drawer.
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
