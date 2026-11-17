import Foundation

struct NavigationMachine {
  enum Phase: Equatable {
    case rest
    case dragging
    case settling(Screen)
  }

  static let response = 0.34
  private static let rest = 0.0015
  private static let dimPeak = 0.22
  private static let filesPosition = Double(Screen.files.rawValue)
  private static let idePosition = Double(Screen.ide.rawValue)
  private static let filesGap = Screen.ide.rawValue
  private static let dragGaps = 0...filesGap
  private static let halfway = 0.5
  private static let angularFrequency = (Double.pi + Double.pi) / response
  private static let dampingFactor = 2.0

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

  init(
    width: Double,
    strip: Double
  ) {
    self.width = width
    self.strip = strip
  }

  var isMoving: Bool { phase != .rest }
  var isAwaitingWeb: Bool { acked < sent }

  private func span(_ gap: Int) -> Double { gap == 1 ? width : width - strip }
  private var atScreen: Bool { abs(position - position.rounded()) < Motion.settleTolerance }

  mutating func beginDrag(toward: Swipe) -> [NavigationCommand]? {
    let from = Int(position.rounded())
    let next = atScreen ? (toward == .rightward ? from - 1 : from) : Int(position.rounded(.down))
    guard Self.dragGaps.contains(next) else { return nil }
    gap = next
    origin = position
    velocity = 0
    phase = .dragging
    guard gap == 1, toward == .leftward else { return [] }
    let frozen = freezeIfNeeded()
    let told = tell(.ide)
    return frozen + told
  }

  mutating func drag(translation: Double) {
    guard phase == .dragging else { return }
    position = min(Double(gap + 1), max(Double(gap), origin - translation / span(gap)))
  }

  mutating func endDrag(
    velocity: Double,
    ending: DragEnd
  ) -> [NavigationCommand] {
    guard phase == .dragging else { return [] }
    let units = -velocity / span(gap)
    let thrown = ending == .cancelled ? position : position + units * Motion.throwSeconds
    let landing = thrown - Double(gap) > Self.halfway ? gap + 1 : gap
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

  mutating func web(
    reports screen: Screen,
    ack: Int
  ) -> [NavigationCommand] {
    if ack < acked { sent = ack }
    acked = ack
    web = screen
    if !isAwaitingWeb, screen != intended {
      intended = screen
      follow(screen)
    }
    return releaseSnapshot()
  }

  private mutating func follow(_ screen: Screen) {
    guard phase != .dragging else { return }
    if screen >= .ide, !hasSnapshot, position < Self.idePosition {
      position = Double(screen.rawValue)
      phase = .rest
      return
    }
    start(settling: screen, velocity: 0)
  }

  private mutating func settle(
    to screen: Screen,
    velocity: Double
  ) -> [NavigationCommand] {
    var commands = screen >= .ide ? freezeIfNeeded() : []
    commands += tell(screen)
    start(settling: screen, velocity: velocity)
    return commands + releaseSnapshot()
  }

  private mutating func start(
    settling screen: Screen,
    velocity: Double
  ) {
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

  mutating func tick(_ elapsed: Double) -> [NavigationCommand] {
    guard case .settling(let screen) = phase else { return [] }
    let target = Double(screen.rawValue)
    let omega = Self.angularFrequency
    var remaining = elapsed
    while remaining > 0 {
      let step = min(remaining, 1.0 / Motion.stepsPerSecond)
      let accel = -omega * omega * (position - target) - Self.dampingFactor * omega * velocity
      velocity += accel * step
      position += velocity * step
      remaining -= step
    }
    if position < travel.lowerBound || position > travel.upperBound {
      position = min(travel.upperBound, max(travel.lowerBound, position))
      velocity = 0
    }
    if abs(position - target) < Self.rest, abs(velocity) < Motion.atRestVelocity {
      arrive(at: screen)
    }
    return releaseSnapshot()
  }

  private mutating func tell(_ screen: Screen) -> [NavigationCommand] {
    let direction = intended < screen ? 1 : -1
    let raws = stride(from: intended.rawValue + direction, through: screen.rawValue, by: direction)
    let screens = raws.compactMap { Screen(rawValue: $0) }
    return screens.map { next in
      sent += 1
      let command = NavigationCommand.send(Self.intent(from: intended, to: next), seq: sent)
      intended = next
      return command
    }
  }

  private static func intent(
    from: Screen,
    to target: Screen
  ) -> String {
    switch (from, target) {
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
    let slide = position
    let chatOffset: Double
    switch slide {
    case ..<1: chatOffset = (1 - slide) * (width - strip)
    case ..<Self.idePosition: chatOffset = -(slide - 1) * width
    default: chatOffset = -width
    }
    let ideOffset = slide <= Self.idePosition ? 0 : -(slide - Self.idePosition) * (width - strip)
    let gap = activeGap
    let shade = shading(gap: gap)
    return Frames(
      gap: gap,
      chatOffset: chatOffset,
      ideOffset: ideOffset,
      amount: shade.amount,
      dim: shade.dim,
      shadow: Motion.shadowAlpha * min(1, shade.amount * Motion.rampFactor),
      chatHeaderAlpha: slide >= Self.idePosition ? 0 : min(1, slide),
      ideHeaderAlpha: ideHeaderAlpha
    )
  }

  private var activeGap: Int? {
    let isDrawerFree = atScreen && (1...Self.idePosition).contains(position.rounded())
    return isDrawerFree ? nil : max(0, min(Self.filesGap, Int(position.rounded(.down))))
  }

  private var ideHeaderAlpha: Double {
    switch position {
    case ...1: return 0
    case ...Self.idePosition: return 1
    default: return Self.filesPosition - position
    }
  }

  private func shading(
    gap: Int?
  ) -> (
    amount: Double,
    dim: Double
  ) {
    guard let gap else { return (0, 0) }
    switch gap {
    case 0: return (1 - position, Self.dimPeak * position)
    case 1: return (position - 1, Self.dimPeak * (Self.idePosition - position))
    default: return (position - Self.idePosition, Self.dimPeak * (Self.filesPosition - position))
    }
  }
}
