import SwiftUI

struct PathParser {
  private static let curveNumberCount = 6
  var path = Path()
  private var numbers: [CGFloat] = []
  private var command: Character = "M"

  private var points: [CGPoint] {
    stride(from: 0, to: numbers.count - 1, by: 2)
      .map {
        CGPoint(x: numbers[$0], y: numbers[$0 + 1])
      }
  }

  mutating func flush() {
    switch (command, numbers.count) {
    case ("M", 2): path.move(to: points[0])
    case ("L", 2): path.addLine(to: points[0])
    case ("C", Self.curveNumberCount): path.addCurve(to: points[2], control1: points[0], control2: points[1])
    case ("Z", _): path.closeSubpath()
    default: break
    }
    numbers = []
  }

  mutating func feed(_ token: Substring) {
    guard let first = token.first else { return }
    guard first.isLetter else { return append(token) }
    flush()
    command = first
    if first == "Z" { flush() }
    append(token.dropFirst())
  }

  private mutating func append(_ token: Substring) {
    guard let value = Double(token) else { return }
    numbers.append(CGFloat(value))
  }
}
