import CoreGraphics

struct EffortDial: Equatable {
  private(set) var pending: String?

  func shown(handed: String?) -> String? { pending ?? handed }

  mutating func release(on level: String, handed: String?) -> Bool {
    guard level != shown(handed: handed) else { return false }
    pending = level
    return true
  }

  mutating func receive(handed: String?, levels: [String]) {
    guard let pending, handed == pending || !levels.contains(pending) else { return }
    self.pending = nil
  }

  static func label(_ level: String) -> String {
    level == "xhigh" ? "XHigh" : level.prefix(1).uppercased() + level.dropFirst()
  }

  static func index(at x: CGFloat, width: CGFloat, count: Int) -> Int {
    guard count > 0, width > 0 else { return 0 }
    return min(count - 1, max(0, Int(x / width * CGFloat(count))))
  }
}
