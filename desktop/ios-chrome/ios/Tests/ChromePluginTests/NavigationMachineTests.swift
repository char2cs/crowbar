import XCTest

@testable import tauri_plugin_crowbar_chrome

/// A stand-in for the web: it applies our intents in order and reports back what it then shows,
/// as `useMobileNativeChrome` does, only when the test lets it.
private struct FakeWeb {
  var screen = Screen.chat
  var ack = 0
  var inbox: [(String, Int)] = []

  mutating func receive(_ commands: [NavigationCommand]) {
    for case .send(let name, let seq) in commands { inbox.append((name, seq)) }
  }

  /// Applies the next `count` intents and returns the report the web would push after them.
  mutating func apply(_ count: Int = .max) -> (Screen, Int) {
    for (name, seq) in inbox.prefix(count) {
      switch name {
      case "openChatList": screen = .chatList
      case "closeChatList": screen = .chat
      case "openIde": screen = .ide
      case "closeIde": screen = .chat
      case "openFiles": screen = .files
      default: screen = .ide
      }
      ack = seq
    }
    inbox.removeFirst(min(count, inbox.count))
    return (screen, ack)
  }
}

final class NavigationMachineTests: XCTestCase {
  private let width = 400.0
  private let strip = 72.0
  private var machine = NavigationMachine(width: 400, strip: 72)
  private var web = FakeWeb()

  override func setUp() {
    machine = NavigationMachine(width: width, strip: strip)
    web = FakeWeb()
  }

  private func run(_ commands: [NavigationCommand]?) {
    web.receive(commands ?? [])
  }

  /// A full swipe: down, travel, up with a fling.
  private func swipe(_ toward: Swipe, distance: Double, velocity: Double) {
    guard let began = machine.beginDrag(toward: toward) else { return }
    run(began)
    machine.drag(translation: toward == .rightward ? distance : -distance)
    run(machine.endDrag(velocity: toward == .rightward ? velocity : -velocity, cancelled: false))
  }

  private func settleFully() {
    var steps = 0
    while machine.isMoving, steps < 600 {
      run(machine.tick(1.0 / 120))
      steps += 1
    }
  }

  private func report(_ count: Int = .max) {
    let (screen, ack) = web.apply(count)
    run(machine.web(reports: screen, ack: ack))
  }

  /// The headers drawn at rest: the chat's, the IDE's, never both.
  private func visibleHeaders() -> Int {
    let frames = machine.frames
    return (frames.chatHeaderAlpha > 0 ? 1 : 0) + (frames.ideHeaderAlpha > 0 ? 1 : 0)
  }

  func testRapidLeftRightLeftLeavesOneScreenAndOneHeader() {
    // Left to the IDE, back to the chat, left again, each before the web has answered.
    swipe(.leftward, distance: 300, velocity: 900)
    swipe(.rightward, distance: 250, velocity: 900)
    swipe(.leftward, distance: 300, velocity: 900)
    report(1)
    report(1)
    report()
    settleFully()
    report()

    XCTAssertEqual(machine.position, 2)
    XCTAssertEqual(machine.phase, .rest)
    XCTAssertEqual(web.screen, .ide)
    XCTAssertEqual(machine.intended, .ide)
    XCTAssertEqual(visibleHeaders(), 1)
    XCTAssertEqual(machine.frames.ideHeaderAlpha, 1)
    XCTAssertNil(machine.frames.gap)
  }

  func testRapidReversalsNeverShowTwoHeadersAtRest() {
    var seed: UInt64 = 0x9E37_79B9_7F4A_7C15
    func next() -> Double {
      seed ^= seed << 13
      seed ^= seed >> 7
      seed ^= seed << 17
      return Double(seed % 10_000) / 10_000
    }
    for _ in 0..<400 {
      switch Int(next() * 5) {
      case 0, 1:
        swipe(next() < 0.5 ? .leftward : .rightward, distance: next() * 380, velocity: next() * 1500)
      case 2:
        for _ in 0..<Int(next() * 30) { run(machine.tick(1.0 / 120)) }
      case 3:
        report(Int(next() * 3))
      default:
        run(machine.closeDrawer())
      }
      XCTAssertTrue((0.0...3.0).contains(machine.position))
    }
    report()
    settleFully()
    report()
    settleFully()

    XCTAssertEqual(machine.phase, .rest)
    XCTAssertEqual(machine.position, Double(web.screen.rawValue))
    XCTAssertEqual(machine.intended, web.screen)
    XCTAssertEqual(machine.hasSnapshot, web.screen >= .ide)
    // A drawer fully out has its own header (the page's); the chat's and IDE's are away.
    XCTAssertEqual(visibleHeaders(), [.chatList, .files].contains(web.screen) ? 0 : 1)
  }

  func testAStaleReportDoesNotTurnTheSwipeAround() {
    swipe(.leftward, distance: 300, velocity: 900)
    // The web still shows the chat: it has not applied openIde yet.
    run(machine.web(reports: .chat, ack: 0))
    XCTAssertEqual(machine.phase, .settling(.ide))
    XCTAssertTrue(machine.isAwaitingWeb)
    settleFully()
    XCTAssertEqual(machine.position, 2)
  }

  func testGrabbingAMovingCardContinuesFromWhereItIs() {
    swipe(.leftward, distance: 300, velocity: 900)
    for _ in 0..<12 { run(machine.tick(1.0 / 120)) }
    let before = machine.position
    XCTAssertGreaterThan(before, 1)
    XCTAssertLessThan(before, 2)

    XCTAssertNotNil(machine.beginDrag(toward: .rightward))
    XCTAssertEqual(machine.position, before)
    machine.drag(translation: 0)
    XCTAssertEqual(machine.position, before)
  }

  func testReversalHandsTheWebTheNewestWordAndKeepsTheChatUntilItAnswers() {
    swipe(.leftward, distance: 300, velocity: 900)
    for _ in 0..<6 { run(machine.tick(1.0 / 120)) }
    swipe(.rightward, distance: 0, velocity: 1200)
    XCTAssertEqual(web.inbox.map(\.0), ["openIde", "closeIde"])

    settleFully()
    XCTAssertEqual(machine.position, 1)
    // Home again, but the web has not answered: the frozen chat still covers the screen.
    XCTAssertTrue(machine.hasSnapshot)

    report(1)
    XCTAssertTrue(machine.hasSnapshot)
    report()
    XCTAssertFalse(machine.hasSnapshot)
    XCTAssertEqual(visibleHeaders(), 1)
  }

  func testTheWebMovingOnItsOwnIsFollowed() {
    run(machine.web(reports: .chatList, ack: 0))
    XCTAssertEqual(machine.phase, .settling(.chatList))
    settleFully()
    XCTAssertEqual(machine.position, 0)
    XCTAssertEqual(visibleHeaders(), 0)
  }

  func testTappingTheIDEButtonFreezesTheChatFirst() {
    XCTAssertEqual(machine.willOpenIde(), [.freezeChat])
    run(machine.web(reports: .ide, ack: 0))
    XCTAssertEqual(machine.phase, .settling(.ide))
    settleFully()
    XCTAssertEqual(machine.position, 2)
  }

  func testAnIDEWithNoFrozenChatIsShownAtOnce() {
    run(machine.web(reports: .ide, ack: 0))
    XCTAssertEqual(machine.position, 2)
    XCTAssertEqual(machine.phase, .rest)
  }

  func testARestartedWebStartsNumberingAgain() {
    swipe(.leftward, distance: 300, velocity: 900)
    swipe(.rightward, distance: 0, velocity: 900)
    XCTAssertTrue(machine.isAwaitingWeb)
    _ = machine.web(reports: .chat, ack: 1)
    // The page reloaded: it reports the chat with nothing applied, and is believed.
    run(machine.web(reports: .chat, ack: 0))
    XCTAssertFalse(machine.isAwaitingWeb)
    settleFully()
    XCTAssertEqual(machine.position, 1)
    XCTAssertFalse(machine.hasSnapshot)
  }

  func testTheDrawerStripPutsTheCardBack() {
    swipe(.rightward, distance: 300, velocity: 900)
    settleFully()
    XCTAssertEqual(machine.position, 0)
    run(machine.closeDrawer())
    XCTAssertEqual(web.inbox.map(\.0), ["openChatList", "closeChatList"])
    settleFully()
    XCTAssertEqual(machine.position, 1)
  }

  func testNoScreenLiesBeyondTheEnds() {
    XCTAssertNotNil(machine.beginDrag(toward: .rightward))
    machine.drag(translation: 0)
    _ = machine.endDrag(velocity: 0, cancelled: true)
    settleFully()
    swipe(.rightward, distance: 400, velocity: 900)
    settleFully()
    XCTAssertEqual(machine.position, 0)
    XCTAssertNil(machine.beginDrag(toward: .rightward))
  }

  func testFramesPlaceEachCardAndKeepTheScrimOffAtRest() {
    XCTAssertEqual(machine.frames.dim, 0)
    XCTAssertEqual(machine.frames.chatOffset, 0)

    run(machine.beginDrag(toward: .rightward))
    machine.drag(translation: 100)
    var frames = machine.frames
    XCTAssertEqual(frames.gap, 0)
    XCTAssertEqual(frames.chatOffset, 100, accuracy: 1e-9)
    XCTAssertEqual(frames.ideOffset, 0)
    XCTAssertEqual(frames.ideHeaderAlpha, 0)
    XCTAssertGreaterThan(frames.dim, 0)

    machine.drag(translation: 328)
    frames = machine.frames
    XCTAssertEqual(frames.chatOffset, 328, accuracy: 1e-9)
    XCTAssertEqual(frames.dim, 0, accuracy: 1e-9)
    XCTAssertEqual(frames.chatHeaderAlpha, 0, accuracy: 1e-9)
  }

  func testFramesSlideTheIDEAwayFromTheFilesWithItsHeader() {
    swipe(.leftward, distance: 400, velocity: 900)
    settleFully()
    report()
    run(machine.beginDrag(toward: .leftward))
    machine.drag(translation: -164)
    let frames = machine.frames
    XCTAssertEqual(frames.gap, 2)
    XCTAssertEqual(frames.ideOffset, -164, accuracy: 1e-9)
    XCTAssertEqual(frames.chatOffset, -400)
    // The header leaves with its card: the strip left of the files shows no buttons.
    XCTAssertEqual(frames.ideHeaderAlpha, 1 - 164.0 / 328.0, accuracy: 1e-9)
    XCTAssertEqual(frames.chatHeaderAlpha, 0)
    machine.drag(translation: -328)
    XCTAssertEqual(machine.frames.ideHeaderAlpha, 0, accuracy: 1e-9)
  }

  func testSettlingDoesNotOvershootIntoTheNextScreen() {
    swipe(.leftward, distance: 380, velocity: 6000)
    var peak = machine.position
    while machine.isMoving {
      run(machine.tick(1.0 / 120))
      peak = max(peak, machine.position)
    }
    XCTAssertLessThanOrEqual(peak, 2)
  }

  func testASettleIsDoneInUnderAHalfSecond() {
    swipe(.leftward, distance: 300, velocity: 900)
    var elapsed = 0.0
    while machine.isMoving {
      run(machine.tick(1.0 / 120))
      elapsed += 1.0 / 120
    }
    XCTAssertLessThan(elapsed, 0.6)
  }
}
