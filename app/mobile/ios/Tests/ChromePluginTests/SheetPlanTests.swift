import XCTest

@testable import tauri_plugin_crowbar_chrome

final class SheetPlanTests: XCTestCase {
  private enum Event {
    case update(SheetOpening)
    case finished
    case noRoot
    case dismissedByUser
  }

  private struct Case {
    let name: String
    let events: [(Event, SheetPlan.Step?)]
  }

  private static let shown: [(Event, SheetPlan.Step?)] = [
    (.update(.open), .present),
    (.finished, SheetPlan.Step.none)]

  private let cases = [
    Case(
      name: "stale open after a user dismissal does not re-present",
      events: Self.shown + [
        (.dismissedByUser, nil),
        (.update(.open), SheetPlan.Step.none),
        (.update(.closed), SheetPlan.Step.none),
        (.update(.open), .present)]
    ),
    Case(
      name: "a web close dismisses once",
      events: Self.shown + [
        (.update(.closed), .dismiss),
        (.update(.closed), SheetPlan.Step.none),
        (.finished, SheetPlan.Step.none)]
    ),
    Case(
      name: "flips during the animation settle to the last value",
      events: [
        (.update(.open), .present),
        (.update(.closed), SheetPlan.Step.none),
        (.finished, .dismiss),
        (.update(.open), SheetPlan.Step.none),
        (.finished, .present)]
    ),
    Case(
      name: "open with no root waits for one",
      events: [
        (.update(.open), .present),
        (.noRoot, SheetPlan.Step.none),
        (.update(.open), .present),
        (.finished, SheetPlan.Step.none),
        (.update(.open), SheetPlan.Step.none)]
    ),
    Case(
      name: "closing while waiting for a root presents nothing",
      events: [
        (.update(.open), .present),
        (.noRoot, SheetPlan.Step.none),
        (.update(.closed), SheetPlan.Step.none),
        (.update(.open), .present)]
    )]

  func testEachScenarioFollowsItsPlan() {
    cases.forEach(run)
  }

  private func run(_ item: Case) {
    var plan = SheetPlan()
    for (event, expected) in item.events {
      XCTAssertEqual(apply(event, to: &plan), expected, item.name)
    }
  }

  private func apply(
    _ event: Event,
    to plan: inout SheetPlan
  ) -> SheetPlan.Step? {
    switch event {
    case .update(let opening): return plan.update(opening)
    case .finished: return plan.finished()
    case .noRoot: return plan.noRoot()
    case .dismissedByUser:
      plan.dismissedByUser()
      return nil
    }
  }
}
