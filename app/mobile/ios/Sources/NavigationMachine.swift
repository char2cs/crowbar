import Foundation

struct NavigationMachine {
  enum Phase: Equatable {
    case rest
    case dragging
    case settling(Screen)
  }

  static let response = 0.34
  private static let rest = 0.0015

  var width: Double
  var strip: Double

  private(set) var position = 1.0
  private(set) var phase = Phase.rest
  private(set) var web = Screen.chat
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
  var isAwaitingWeb: Bool { acked < sent }

  private func span(_ gap: Int) -> Double { gap == 1 ? width : width - strip }
  private var atScreen: Bool { abs(position - position.rounded()) < 1e-9 }

  mutating func beginDrag(toward: Swipe) -> [NavigationCommand]? {
    let from = Int(position.rounded())
    let next = atScreen ? (toward == .rightward ? from - 1 : from) : Int(position.rounded(.down))
    guard (0...2).contains(next) else { return nil }
    gap = next
    origin = position
    velocity = 0
    phase = .dragging
    var commands: [NavigationCommand] = []
    if gap == 1, toward == .leftward {
      commands += freezeIfNeeded()
      commands += tell(.ide)
    }
    return commands
  }

  mutating func drag(translation: Double) {
    guard phase == .dragging else { return }
    position = min(Double(gap + 1), max(Double(gap), origin - translation / span(gap)))
  }

  mutating func endDrag(velocity: Double, ending: DragEnd) -> [NavigationCommand] {
    guard phase == .dragging else { return [] }
    let units = -velocity / span(gap)
    let thrown = ending == .cancelled ? position : position + units * 0.15
    let landing = thrown - Double(gap) > 0.5 ? gap + 1 : gap
    return settle(to: Screen(rawValue: landing) ?? .chat, velocity: units)
  }

  mutating func closeDrawer() -> [NavigationCommand] {
    guard atScreen, let screen = Screen(rawValue: Int(position.rounded())) else { return [] }
    switch screen {
    case .chatList: return settle(to: .chat, velocity: 0)
    case .files: return settle(to: .ide, velocity: 0)
    default: return []
    }
  }

  mutating func willOpenIde() -> [NavigationCommand] {
    position <= 1 ? freezeIfNeeded() : []
  }

  mutating func web(reports screen: Screen, ack: Int) -> [NavigationCommand] {
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

  private mutating func follow(_ screen: Screen) {
    guard phase != .dragging else { return }
    if screen >= .ide, !hasSnapshot, position < 2 {
      position = Double(screen.rawValue)
      phase = .rest
      return
    }
    start(settling: screen, velocity: 0)
  }

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

  private mutating func tell(_ screen: Screen) -> [NavigationCommand] {
    var commands: [NavigationCommand] = []
    while intended != screen {
      guard let next = Screen(rawValue: intended.rawValue + (intended < screen ? 1 : -1)) else {
        break
      }
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

  private mutating func releaseSnapshot() -> [NavigationCommand] {
    guard hasSnapshot, phase == .rest, position == 1, web == .chat, !isAwaitingWeb else {
      return []
    }
    hasSnapshot = false
    return [.dropSnapshot]
  }

  var frames: Frames {
    let x = position
    let chatOffset: Double
    switch x {
    case ..<1: chatOffset = (1 - x) * (width - strip)
    case ..<2: chatOffset = -(x - 1) * width
    default: chatOffset = -width
    }
    let ideOffset = x <= 2 ? 0 : -(x - 2) * (width - strip)
    let gap = atScreen && (1...2).contains(x.rounded()) ? nil : (x < 1 ? 0 : x < 2 ? 1 : 2)
    let amount: Double
    let dim: Double
    switch gap {
    case 0: (amount, dim) = (1 - x, 0.22 * x)
    case 1: (amount, dim) = (x - 1, 0.22 * (2 - x))
    case 2: (amount, dim) = (x - 2, 0.22 * (3 - x))
    default: (amount, dim) = (0, 0)
    }
    return Frames(
      gap: gap,
      chatOffset: chatOffset,
      ideOffset: ideOffset,
      amount: amount,
      dim: dim,
      shadow: 0.18 * min(1, amount * 4),
      chatHeaderAlpha: x >= 2 ? 0 : min(1, x),
      ideHeaderAlpha: x <= 1 ? 0 : x <= 2 ? 1 : 3 - x
    )
  }
}
