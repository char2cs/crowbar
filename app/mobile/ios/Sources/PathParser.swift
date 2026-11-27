import SwiftUI

struct PathParser {
  private static let pointNumberCount = 2
  private static let curveNumberCount = 6
  private static let curveEndIndex = 2
  var path = Path()
  private var numbers: [CGFloat] = []
  private var command: Character = "M"

  private var points: [CGPoint] {
    stride(from: 0, to: numbers.count - 1, by: Self.pointNumberCount)
      .map {
        CGPoint(x: numbers[$0], y: numbers[$0 + 1])
      }
  }

  mutating func flush() {
    switch (command, numbers.count) {
    case ("M", Self.pointNumberCount): path.move(to: points[0])
    case ("L", Self.pointNumberCount): path.addLine(to: points[0])
    case ("C", Self.curveNumberCount): addCurve()
    case ("Z", _): path.closeSubpath()
    default: break
    }
    numbers = []
  }

  private mutating func addCurve() {
    path.addCurve(to: points[Self.curveEndIndex], control1: points[0], control2: points[1])
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
