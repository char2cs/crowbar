import XCTest

@testable import tauri_plugin_crowbar_chrome

final class EffortDialTests: XCTestCase {
  private let levels = ["low", "medium", "high", "xhigh", "max"]

  // The recorded flip-flop: release on Low while Max is handed, the old Max arrives once more,
  // then Low. The control must show Low throughout.
  func testStaleEchoOfThePreviousLevelDoesNotMoveThePill() {
    var dial = EffortDial()
    XCTAssertTrue(dial.release(on: "low", handed: "max"))
    XCTAssertEqual(dial.shown(handed: "max"), "low")
    dial.receive(handed: "max", levels: levels)
    XCTAssertEqual(dial.shown(handed: "max"), "low")
    dial.receive(handed: "low", levels: levels)
    XCTAssertNil(dial.pending)
    XCTAssertEqual(dial.shown(handed: "low"), "low")
  }

  func testReleasingOnTheShownLevelReportsNothing() {
    var dial = EffortDial()
    XCTAssertFalse(dial.release(on: "max", handed: "max"))
    XCTAssertTrue(dial.release(on: "low", handed: "max"))
    XCTAssertFalse(dial.release(on: "low", handed: "max"))
  }

  func testAModelSwitchThatDropsTheLevelEndsTheWait() {
    var dial = EffortDial()
    _ = dial.release(on: "max", handed: "low")
    dial.receive(handed: "low", levels: ["low", "medium", "high"])
    XCTAssertEqual(dial.shown(handed: "low"), "low")
  }

  func testIndexClampsToTheTrack() {
    XCTAssertEqual(EffortDial.index(at: -20, width: 300, count: 5), 0)
    XCTAssertEqual(EffortDial.index(at: 119, width: 300, count: 5), 1)
    XCTAssertEqual(EffortDial.index(at: 120, width: 300, count: 5), 2)
    XCTAssertEqual(EffortDial.index(at: 999, width: 300, count: 5), 4)
    XCTAssertEqual(EffortDial.index(at: 10, width: 0, count: 5), 0)
  }
}
