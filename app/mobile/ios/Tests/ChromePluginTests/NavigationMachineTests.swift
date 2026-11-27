import XCTest

@testable import tauri_plugin_crowbar_chrome

final class NavigationMachineTests: XCTestCase {
  private var rig = NavigationRig()

  override func setUp() {
    rig = NavigationRig()
  }

  func testRapidLeftRightLeftLeavesOneScreenAndOneHeader() {
    rig.swipe(.leftward, distance: 300, velocity: 900)
    rig.swipe(.rightward, distance: 250, velocity: 900)
    rig.swipe(.leftward, distance: 300, velocity: 900)
    rig.report(1)
    rig.report(1)
    rig.report()
    rig.settleFully()
    rig.report()

    XCTAssertEqual(rig.machine.position, 2)
    XCTAssertEqual(rig.machine.phase, .rest)
    XCTAssertEqual(rig.web.screen, .ide)
    XCTAssertEqual(rig.machine.intended, .ide)
    XCTAssertEqual(rig.visibleHeaders(), 1)
    XCTAssertEqual(rig.machine.frames.ideHeaderAlpha, 1)
    XCTAssertNil(rig.machine.frames.gap)
  }

  private func fuzz(steps: Int) {
    var seed: UInt64 = 0x9E37_79B9_7F4A_7C15
    func next() -> Double {
      seed ^= seed << 13
      seed ^= seed >> 7
      seed ^= seed << 17
      return Double(seed % 10_000) / 10_000
    }
    for _ in 0..<steps {
      switch Int(next() * 5) {
      case 0, 1:
        rig.swipe(
          next() < 0.5 ? .leftward : .rightward,
          distance: next() * 380,
          velocity: next() * 1500
        )
      case 2:
        for _ in 0..<Int(next() * 30) { rig.run(rig.machine.tick(1.0 / 120)) }
      case 3:
        rig.report(Int(next() * 3))
      default:
        rig.run(rig.machine.closeDrawer())
      }
      XCTAssertTrue((0.0...3.0).contains(rig.machine.position))
    }
  }

  func testRapidReversalsNeverShowTwoHeadersAtRest() {
    fuzz(steps: 400)
    rig.report()
    rig.settleFully()
    rig.report()
    rig.settleFully()

    XCTAssertEqual(rig.machine.phase, .rest)
    XCTAssertEqual(rig.machine.position, Double(rig.web.screen.rawValue))
    XCTAssertEqual(rig.machine.intended, rig.web.screen)
    XCTAssertEqual(rig.machine.hasSnapshot, rig.web.screen >= .ide)
    XCTAssertEqual(rig.visibleHeaders(), [.chatList, .files].contains(rig.web.screen) ? 0 : 1)
  }

  func testAStaleReportDoesNotTurnTheSwipeAround() {
    rig.swipe(.leftward, distance: 300, velocity: 900)
    rig.run(rig.machine.web(reports: .chat, ack: 0))
    XCTAssertEqual(rig.machine.phase, .settling(.ide))
    XCTAssertTrue(rig.machine.isAwaitingWeb)
    rig.settleFully()
    XCTAssertEqual(rig.machine.position, 2)
  }

  func testGrabbingAMovingCardContinuesFromWhereItIs() {
    rig.swipe(.leftward, distance: 300, velocity: 900)
    for _ in 0..<12 { rig.run(rig.machine.tick(1.0 / 120)) }
    let before = rig.machine.position
    XCTAssertGreaterThan(before, 1)
    XCTAssertLessThan(before, 2)

    XCTAssertNotNil(rig.machine.beginDrag(toward: .rightward))
    XCTAssertEqual(rig.machine.position, before)
    rig.machine.drag(translation: 0)
    XCTAssertEqual(rig.machine.position, before)
  }

  func testReversalHandsTheWebTheNewestWordAndKeepsTheChatUntilItAnswers() {
    rig.swipe(.leftward, distance: 300, velocity: 900)
    for _ in 0..<6 { rig.run(rig.machine.tick(1.0 / 120)) }
    rig.swipe(.rightward, distance: 0, velocity: 1200)
    XCTAssertEqual(rig.web.inbox.map(\.0), ["openIde", "closeIde"])

    rig.settleFully()
    XCTAssertEqual(rig.machine.position, 1)
    XCTAssertTrue(rig.machine.hasSnapshot)

    rig.report(1)
    XCTAssertTrue(rig.machine.hasSnapshot)
    rig.report()
    XCTAssertFalse(rig.machine.hasSnapshot)
    XCTAssertEqual(rig.visibleHeaders(), 1)
  }

  func testTheWebMovingOnItsOwnIsFollowed() {
    rig.run(rig.machine.web(reports: .chatList, ack: 0))
    XCTAssertEqual(rig.machine.phase, .settling(.chatList))
    rig.settleFully()
    XCTAssertEqual(rig.machine.position, 0)
    XCTAssertEqual(rig.visibleHeaders(), 0)
  }

  func testTappingTheIDEButtonFreezesTheChatFirst() {
    XCTAssertEqual(rig.machine.willOpenIde(), [.freezeChat])
    rig.run(rig.machine.web(reports: .ide, ack: 0))
    XCTAssertEqual(rig.machine.phase, .settling(.ide))
    rig.settleFully()
    XCTAssertEqual(rig.machine.position, 2)
  }

  func testAnIDEWithNoFrozenChatIsShownAtOnce() {
    rig.run(rig.machine.web(reports: .ide, ack: 0))
    XCTAssertEqual(rig.machine.position, 2)
    XCTAssertEqual(rig.machine.phase, .rest)
  }

  func testARestartedWebStartsNumberingAgain() {
    rig.swipe(.leftward, distance: 300, velocity: 900)
    rig.swipe(.rightward, distance: 0, velocity: 900)
    XCTAssertTrue(rig.machine.isAwaitingWeb)
    _ = rig.machine.web(reports: .chat, ack: 1)
    rig.run(rig.machine.web(reports: .chat, ack: 0))
    XCTAssertFalse(rig.machine.isAwaitingWeb)
    rig.settleFully()
    XCTAssertEqual(rig.machine.position, 1)
    XCTAssertFalse(rig.machine.hasSnapshot)
  }

  func testTheDrawerStripPutsTheCardBack() {
    rig.swipe(.rightward, distance: 300, velocity: 900)
    rig.settleFully()
    XCTAssertEqual(rig.machine.position, 0)
    rig.run(rig.machine.closeDrawer())
    XCTAssertEqual(rig.web.inbox.map(\.0), ["openChatList", "closeChatList"])
    rig.settleFully()
    XCTAssertEqual(rig.machine.position, 1)
  }

  func testNoScreenLiesBeyondTheEnds() {
    XCTAssertNotNil(rig.machine.beginDrag(toward: .rightward))
    rig.machine.drag(translation: 0)
    _ = rig.machine.endDrag(velocity: 0, ending: .cancelled)
    rig.settleFully()
    rig.swipe(.rightward, distance: 400, velocity: 900)
    rig.settleFully()
    XCTAssertEqual(rig.machine.position, 0)
    XCTAssertNil(rig.machine.beginDrag(toward: .rightward))
  }

  func testFramesPlaceEachCardAndKeepTheScrimOffAtRest() {
    XCTAssertEqual(rig.machine.frames.dim, 0)
    XCTAssertEqual(rig.machine.frames.chatOffset, 0)

    rig.run(rig.machine.beginDrag(toward: .rightward))
    rig.machine.drag(translation: 100)
    var frames = rig.machine.frames
    XCTAssertEqual(frames.gap, 0)
    XCTAssertEqual(frames.chatOffset, 100, accuracy: 1e-9)
    XCTAssertEqual(frames.ideOffset, 0)
    XCTAssertEqual(frames.ideHeaderAlpha, 0)
    XCTAssertGreaterThan(frames.dim, 0)

    rig.machine.drag(translation: 328)
    frames = rig.machine.frames
    XCTAssertEqual(frames.chatOffset, 328, accuracy: 1e-9)
    XCTAssertEqual(frames.dim, 0, accuracy: 1e-9)
    XCTAssertEqual(frames.chatHeaderAlpha, 0, accuracy: 1e-9)
  }

  private enum Aftermath { case settle, settleAndReport }

  private struct DrawerCase {
    let name: String
    let swipes: [(toward: Swipe, then: Aftermath)]
    let position: Double
    let gap: Int
    var amount: Double?
  }

  private let chatListCase = DrawerCase(
    name: "the chat list rests open",
    swipes: [(.rightward, .settle)],
    position: 0,
    gap: 0,
    amount: 1
  )

  private let filesCase = DrawerCase(
    name: "the files drawer rests open",
    swipes: [(.leftward, .settleAndReport), (.leftward, .settleAndReport)],
    position: 3,
    gap: 2
  )

  private var drawerCases: [DrawerCase] { [chatListCase, filesCase] }

  func testShadowStaysOnTheSlidAsideCardWhenADrawerRestsOpen() {
    for item in drawerCases {
      setUp()
      XCTAssertEqual(rig.machine.frames.shadow, 0, item.name)
      for step in item.swipes { fling(step) }
      XCTAssertEqual(rig.machine.position, item.position, item.name)
      XCTAssertEqual(rig.machine.frames.gap, item.gap, item.name)
      XCTAssertEqual(rig.machine.frames.shadow, 0.18, accuracy: 1e-9, item.name)
      XCTAssertEqual(rig.machine.frames.amount, item.amount ?? rig.machine.frames.amount, item.name)
    }
  }

  private func fling(_ step: (toward: Swipe, then: Aftermath)) {
    rig.swipe(step.toward, distance: 400, velocity: 900)
    rig.settleFully()
    if step.then == .settleAndReport { rig.report() }
  }

  func testFramesSlideTheIDEAwayFromTheFilesWithItsHeader() {
    rig.swipe(.leftward, distance: 400, velocity: 900)
    rig.settleFully()
    rig.report()
    rig.run(rig.machine.beginDrag(toward: .leftward))
    rig.machine.drag(translation: -164)
    let frames = rig.machine.frames
    XCTAssertEqual(frames.gap, 2)
    XCTAssertEqual(frames.ideOffset, -164, accuracy: 1e-9)
    XCTAssertEqual(frames.chatOffset, -400)
    XCTAssertEqual(frames.ideHeaderAlpha, 1 - 164.0 / 328.0, accuracy: 1e-9)
    XCTAssertEqual(frames.chatHeaderAlpha, 0)
    rig.machine.drag(translation: -328)
    XCTAssertEqual(rig.machine.frames.ideHeaderAlpha, 0, accuracy: 1e-9)
  }

  func testSettlingDoesNotOvershootIntoTheNextScreen() {
    rig.swipe(.leftward, distance: 380, velocity: 6000)
    var peak = rig.machine.position
    while rig.machine.isMoving {
      rig.run(rig.machine.tick(1.0 / 120))
      peak = max(peak, rig.machine.position)
    }
    XCTAssertLessThanOrEqual(peak, 2)
  }

  func testASettleIsDoneInUnderAHalfSecond() {
    rig.swipe(.leftward, distance: 300, velocity: 900)
    var elapsed = 0.0
    while rig.machine.isMoving {
      rig.run(rig.machine.tick(1.0 / 120))
      elapsed += 1.0 / 120
    }
    XCTAssertLessThan(elapsed, 0.6)
  }
}
