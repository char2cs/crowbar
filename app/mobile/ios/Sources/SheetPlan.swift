import Foundation

struct SheetPlan {
  enum Step { case none, present, dismiss }

  private var wantOpen = false
  private(set) var presented = false
  // PLATFORM: A present or dismiss still animating: the web can flip open and closed faster than that
  // PLATFORM: (React's dev double-mount does), so the next step waits for this one's completion.
  private(set) var moving = false
  private var waitingForRoot = false

  mutating func update(_ opening: SheetOpening) -> Step {
    let open = opening == .open
    if open == wantOpen { return waitingForRoot ? next() : .none }
    wantOpen = open
    waitingForRoot = false
    return next()
  }

  mutating func noRoot() -> Step {
    moving = false
    presented = false
    waitingForRoot = true
    return .none
  }

  mutating func finished() -> Step {
    moving = false
    return next()
  }

  mutating func dismissedByUser() { presented = false }

  private mutating func next() -> Step {
    waitingForRoot = false
    guard !moving, wantOpen != presented else { return .none }
    moving = true
    presented = wantOpen
    return wantOpen ? .present : .dismiss
  }
}
