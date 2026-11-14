import Foundation

/// A widget that counts taps.
public struct Widget {
  let taps: Int

  func tapped() -> Widget {
    guard taps < 10 else { return self }
    return Widget(taps: taps + 1)
  }
}

extension Widget: Equatable {}
