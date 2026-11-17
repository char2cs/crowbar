import Foundation

@testable import tauri_plugin_crowbar_chrome

final class NavigationRig {
  private static let width = 400.0
  private static let strip = 72.0
  private static let frameSeconds = 1.0 / 120
  private static let maxSettleFrames = 600

  var machine = NavigationMachine(width: Self.width, strip: Self.strip)
  var web = FakeWeb()

  func run(_ commands: [NavigationCommand]?) {
    web.receive(commands ?? [])
  }

  func swipe(
    _ toward: Swipe,
    distance: Double,
    velocity: Double
  ) {
    guard let began = machine.beginDrag(toward: toward) else { return }
    run(began)
    machine.drag(translation: toward == .rightward ? distance : -distance)
    run(machine.endDrag(velocity: toward == .rightward ? velocity : -velocity, ending: .released))
  }

  func settleFully() {
    var steps = 0
    while machine.isMoving, steps < Self.maxSettleFrames {
      run(machine.tick(Self.frameSeconds))
      steps += 1
    }
  }

  func report(_ count: Int = .max) {
    let (screen, ack) = web.apply(count)
    run(machine.web(reports: screen, ack: ack))
  }

  func visibleHeaders() -> Int {
    let frames = machine.frames
    return (frames.chatHeaderAlpha > 0 ? 1 : 0) + (frames.ideHeaderAlpha > 0 ? 1 : 0)
  }
}
