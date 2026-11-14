import Foundation

struct CSSArgs {
  let parts: [String]
  let alpha: Double

  init(_ text: Substring) {
    let halves = text.split(separator: "/", maxSplits: 1)
    var parts =
      halves.first.map {
        $0.split(whereSeparator: { $0 == "," || $0 == " " }).map(String.init)
      } ?? []
    var alpha = halves.count == 2 ? String(halves[1]).trimmingCharacters(in: .whitespaces) : nil
    if alpha == nil, parts.count == 4, parts.first.flatMap(Double.init) != nil {
      alpha = parts.removeLast()
    }
    self.parts = parts
    self.alpha = alpha.flatMap { Self.value($0) } ?? 1
  }

  func channel(_ index: Int, scale: Double = 1, percentOf: Double = 1) -> Double? {
    guard index < parts.count else { return nil }
    return Self.value(parts[index], scale: scale, percentOf: percentOf)
  }

  func number(_ index: Int) -> Double? {
    guard index < parts.count else { return nil }
    return Double(parts[index].replacingOccurrences(of: "deg", with: ""))
  }

  private static func value(_ part: String, scale: Double = 1, percentOf: Double = 1) -> Double? {
    if part == "none" { return 0 }
    if part.hasSuffix("%") { return Double(part.dropLast()).map { $0 / 100 * percentOf } }
    return Double(part).map { $0 / scale }
  }
}
