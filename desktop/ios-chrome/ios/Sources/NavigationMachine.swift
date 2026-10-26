import Foundation

/// The phone's screens in one line. Between two neighbours a card slides: the chat card right
/// over the chat list, left off the IDE, and the IDE card left off the files.
enum Screen: Int, Comparable {
  case chatList = 0, chat, ide, files

  static func < (a: Screen, b: Screen) -> Bool { a.rawValue < b.rawValue }
}

enum Swipe { case leftward, rightward }

/// What the machine asks of the views and of the web.
enum NavigationCommand: Equatable {
  /// A navigation intent for the web. `seq` comes back as `ack` once the web has applied it.
  case send(String, seq: Int)
  /// Snapshot the chat before the web swaps it for the IDE beneath.
  case freezeChat
  case dropSnapshot
}

/// Where every layer stands at a position.
struct Frames: Equatable {
  /// Which gap the position is in (0 chat list, 1 chat/IDE, 2 IDE/files); nil at rest.
  var gap: Int?
  var chatOffset: Double
  var ideOffset: Double
  /// How far the moving card is from its closed place, 0...1.
  var amount: Double
  var dim: Double
  var chatHeaderAlpha: Double
  var ideHeaderAlpha: Double
}

/// The one owner of what is on screen and in motion: a position along the screens, the drag or
/// spring moving it, and how the web's word relates to ours. Pure: views render `frames`, and
/// every user action goes through here, so a swipe, a reversal or a late report from the web
/// cannot leave two screens, or two headers, in front.
///
/// The web owns which screen is current; this owns the motion. Each intent we send carries a
/// `seq`, and the web echoes the newest one it applied (`ack`). Until that catches up, what the
/// web reports is older than our last word and is not allowed to move us.
struct NavigationMachine {
  enum Phase: Equatable {
    case rest
    case dragging
    case settling(Screen)
  }

  /// Seconds for a settle; critically damped, so it never overshoots into the next screen.
  static let response = 0.34
  private static let rest = 0.0015

  var width: Double
  var strip: Double

  /// 0 chat list, 1 chat, 2 IDE, 3 files; fractional while a card is between screens.
  private(set) var position = 1.0
  private(set) var phase = Phase.rest
  /// What the web shows (its last report). The snapshot stands in for the chat while it is not.
  private(set) var web = Screen.chat
  /// Where the web is headed: our last word, or its own when it moved without us.
  private(set) var intended = Screen.chat
  private(set) var hasSnapshot = false
  private var velocity = 0.0
  private var sent = 0
  private var acked = 0
  private var gap = 0
  private var origin = 1.0
  private var travel = 0.0...3.0

  init(width: Double, strip: Double) {
    self.width = width
    self.strip = strip
  }

  var isMoving: Bool { phase != .rest }
  /// The web's report is older than something we told it.
  var isAwaitingWeb: Bool { acked < sent }

  private func span(_ gap: Int) -> Double { gap == 1 ? width : width - strip }
  private var atScreen: Bool { abs(position - position.rounded()) < 1e-9 }

  // MARK: drag

  /// A finger went down and moved `toward`. Nil when no screen lies that way.
  mutating func beginDrag(toward: Swipe) -> [NavigationCommand]? {
    let from = Int(position.rounded())
    let next = atScreen ? (toward == .rightward ? from - 1 : from) : Int(position.rounded(.down))
    guard (0...2).contains(next) else { return nil }
    gap = next
    origin = position
    velocity = 0
    phase = .dragging
    var commands: [NavigationCommand] = []
    // Toward the IDE it must already be beneath the chat card; the chat is kept to slide away.
    if gap == 1, toward == .leftward {
      commands += freezeIfNeeded()
      commands += tell(.ide)
    }
    return commands
  }

  /// The finger's travel since it went down, in points, positive rightward.
  mutating func drag(translation: Double) {
    guard phase == .dragging else { return }
    position = min(Double(gap + 1), max(Double(gap), origin - translation / span(gap)))
  }

  /// The finger lifted at `velocity` points per second, positive rightward.
  mutating func endDrag(velocity: Double, cancelled: Bool) -> [NavigationCommand] {
    guard phase == .dragging else { return [] }
    let units = -velocity / span(gap)
    let thrown = cancelled ? position : position + units * 0.15
    let landing = thrown - Double(gap) > 0.5 ? gap + 1 : gap
    return settle(to: Screen(rawValue: landing) ?? .chat, velocity: units)
  }

  /// A tap on the strip of a card slid aside puts it back.
  mutating func closeDrawer() -> [NavigationCommand] {
    guard atScreen, let screen = Screen(rawValue: Int(position.rounded())) else { return [] }
    switch screen {
    case .chatList: return settle(to: .chat, velocity: 0)
    case .files: return settle(to: .ide, velocity: 0)
    default: return []
    }
  }

  /// The web is about to put the IDE where the chat was (its button was tapped).
  mutating func willOpenIde() -> [NavigationCommand] {
    position <= 1 ? freezeIfNeeded() : []
  }

  // MARK: the web's word

  /// The web's current screen, with the newest intent seq it had applied when it said so.
  mutating func web(reports screen: Screen, ack: Int) -> [NavigationCommand] {
    // A smaller ack is a web that restarted: its numbering began again, and so does ours.
    if ack < acked { sent = ack }
    acked = ack
    web = screen
    var commands: [NavigationCommand] = []
    if !isAwaitingWeb, screen != intended {
      intended = screen
      follow(screen)
    }
    commands += releaseSnapshot()
    return commands
  }

  /// The web moved on its own (a button, a route): go there, unless a finger is mid-drag, whose
  /// end tells the web where it landed.
  private mutating func follow(_ screen: Screen) {
    guard phase != .dragging else { return }
    // Without the frozen chat nothing can slide away: show the screen at once.
    if screen >= .ide, !hasSnapshot, position < 2 {
      position = Double(screen.rawValue)
      phase = .rest
      return
    }
    start(settling: screen, velocity: 0)
  }

  // MARK: motion

  private mutating func settle(to screen: Screen, velocity: Double) -> [NavigationCommand] {
    var commands = screen >= .ide ? freezeIfNeeded() : []
    commands += tell(screen)
    start(settling: screen, velocity: velocity)
    return commands + releaseSnapshot()
  }

  private mutating func start(settling screen: Screen, velocity: Double) {
    let target = Double(screen.rawValue)
    travel = min(position, target)...max(position, target)
    self.velocity = velocity
    phase = .settling(screen)
    if abs(position - target) < Self.rest { arrive(at: screen) }
  }

  private mutating func arrive(at screen: Screen) {
    position = Double(screen.rawValue)
    velocity = 0
    phase = .rest
  }

  /// Advances a settle by `dt` seconds.
  mutating func tick(_ dt: Double) -> [NavigationCommand] {
    guard case .settling(let screen) = phase else { return [] }
    let target = Double(screen.rawValue)
    let omega = 2 * Double.pi / Self.response
    var remaining = dt
    while remaining > 0 {
      let step = min(remaining, 1.0 / 240)
      let accel = -omega * omega * (position - target) - 2 * omega * velocity
      velocity += accel * step
      position += velocity * step
      remaining -= step
    }
    if position < travel.lowerBound || position > travel.upperBound {
      position = min(travel.upperBound, max(travel.lowerBound, position))
      velocity = 0
    }
    if abs(position - target) < Self.rest, abs(velocity) < 0.05 { arrive(at: screen) }
    return releaseSnapshot()
  }

  // MARK: intents and the chat snapshot

  /// Intents that walk the web from where it is headed to `screen`, one neighbour at a time.
  private mutating func tell(_ screen: Screen) -> [NavigationCommand] {
    var commands: [NavigationCommand] = []
    while intended != screen {
      let next = Screen(rawValue: intended.rawValue + (intended < screen ? 1 : -1))!
      sent += 1
      commands.append(.send(Self.intent(from: intended, to: next), seq: sent))
      intended = next
    }
    return commands
  }

  private static func intent(from: Screen, to: Screen) -> String {
    switch (from, to) {
    case (.chat, .chatList): return "openChatList"
    case (.chatList, .chat): return "closeChatList"
    case (.chat, .ide): return "openIde"
    case (.ide, .chat): return "closeIde"
    case (.ide, .files): return "openFiles"
    default: return "closeFiles"
    }
  }

  private mutating func freezeIfNeeded() -> [NavigationCommand] {
    guard !hasSnapshot else { return [] }
    hasSnapshot = true
    return [.freezeChat]
  }

  /// The frozen chat goes once the live webview shows the chat again and nothing is in flight.
  private mutating func releaseSnapshot() -> [NavigationCommand] {
    guard hasSnapshot, phase == .rest, position == 1, web == .chat, !isAwaitingWeb else {
      return []
    }
    hasSnapshot = false
    return [.dropSnapshot]
  }

  // MARK: layout

  var frames: Frames {
    let x = position
    let chatOffset: Double
    switch x {
    case ..<1: chatOffset = (1 - x) * (width - strip)
    case ..<2: chatOffset = -(x - 1) * width
    default: chatOffset = -width
    }
    let ideOffset = x <= 2 ? 0 : -(x - 2) * (width - strip)
    let gap = atScreen ? nil : (x < 1 ? 0 : x < 2 ? 1 : 2)
    let amount: Double
    let dim: Double
    switch gap {
    case 0: (amount, dim) = (1 - x, 0.22 * x)
    case 1: (amount, dim) = (x - 1, 0.22 * (2 - x))
    case 2: (amount, dim) = (x - 2, 0.22 * (3 - x))
    default: (amount, dim) = (0, 0)
    }
    return Frames(
      gap: gap, chatOffset: chatOffset, ideOffset: ideOffset, amount: amount, dim: dim,
      chatHeaderAlpha: x >= 2 ? 0 : min(1, x),
      ideHeaderAlpha: x <= 1 ? 0 : x <= 2 ? 1 : 3 - x)
  }
}
