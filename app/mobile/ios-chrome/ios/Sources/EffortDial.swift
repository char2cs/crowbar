import CoreGraphics

/// The effort control's one piece of state of its own: the level the finger released on, held
/// only until the web hands a level back. The web owns the value; this keeps the control from
/// drawing an echo of the old one that is still in flight.
struct EffortDial: Equatable {
  private(set) var pending: String?

  /// What the control draws: the release in flight, else what the web last handed over.
  func shown(handed: String?) -> String? { pending ?? handed }

  /// The finger lifted on `level`; true when that is a new choice to report.
  mutating func release(on level: String, handed: String?) -> Bool {
    guard level != shown(handed: handed) else { return false }
    pending = level
    return true
  }

  /// The web handed over `handed` among `levels`. Only the level asked for (or that level
  /// vanishing, as a model switch does) ends the wait; an older value is just late.
  mutating func receive(handed: String?, levels: [String]) {
    guard let pending, handed == pending || !levels.contains(pending) else { return }
    self.pending = nil
  }

  /// The segment under `x` on a track `width` wide split into `count` equal segments.
  static func index(at x: CGFloat, width: CGFloat, count: Int) -> Int {
    guard count > 0, width > 0 else { return 0 }
    return min(count - 1, max(0, Int(x / width * CGFloat(count))))
  }
}
