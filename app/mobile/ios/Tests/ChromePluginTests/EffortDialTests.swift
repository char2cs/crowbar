import XCTest

@testable import tauri_plugin_crowbar_chrome

final class EffortDialTests: XCTestCase {
  private let levels = ["low", "medium", "high", "xhigh", "max"]

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

  func testLabelsReadAsWords() {
    XCTAssertEqual(EffortDial.label("low"), "Low")
    XCTAssertEqual(EffortDial.label("xhigh"), "XHigh")
    XCTAssertEqual(EffortDial.label("max"), "Max")
  }

  private struct StepCase {
    let name: String
    let delta: Int
    let handed: String?
    let expected: String?
  }

  private static func table(_ items: StepCase...) -> [StepCase] { items }

  private static let stepCases = table(
    StepCase(name: "down from medium", delta: -1, handed: "medium", expected: "low"),
    StepCase(name: "down at the bottom", delta: -1, handed: "low", expected: nil),
    StepCase(name: "up at the top", delta: 1, handed: "max", expected: nil),
    StepCase(name: "nothing handed", delta: 1, handed: nil, expected: nil)
  )

  func testSteppingMovesOneLevelAndStopsAtTheEnds() {
    for item in Self.stepCases {
      var dial = EffortDial()
      XCTAssertEqual(
        dial.step(by: item.delta, levels: levels, handed: item.handed),
        item.expected,
        item.name
      )
    }
  }

  func testPickingAtAPositionReleasesThatLevelOnce() {
    var dial = EffortDial()
    let touch = EffortDial.Touch(position: 10, width: 300)
    XCTAssertEqual(dial.pick(at: touch, levels: levels, handed: "max"), "low")
    XCTAssertNil(dial.pick(at: touch, levels: levels, handed: "max"))
  }
}
