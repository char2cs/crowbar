import SwiftUI
import UIKit

extension CrowbarIcon {
  init?(named name: String) { self.init(rawValue: name) }

  func path(in size: CGFloat) -> Path {
    let scale = size / art.viewBox
    return Self.outlines[self, default: Path()].applying(CGAffineTransform(scaleX: scale, y: scale))
  }

  private static let outlines: [CrowbarIcon: Path] = Dictionary(
    uniqueKeysWithValues: allCases.map { icon in
      var outline = Path()
      for data in icon.art.paths { outline.addPath(parse(data)) }
      return (icon, outline)
    }
  )

  func image(size: CGFloat) -> UIImage {
    let key = "\(rawValue)@\(size)"
    if let cached = Self.rendered.object(forKey: key as NSString) { return cached }
    let art = art
    let image = UIGraphicsImageRenderer(size: CGSize(width: size, height: size))
      .image { context in
        UIColor.black.setFill()
        UIColor.black.setStroke()
        let outline = UIBezierPath(cgPath: path(in: size).cgPath)
        if art.stroke > 0 {
          outline.lineWidth = art.stroke * size / art.viewBox
          outline.lineCapStyle = .round
          outline.lineJoinStyle = .round
          outline.stroke()
        } else {
          outline.fill()
        }
      }
      .withRenderingMode(.alwaysTemplate)
    Self.rendered.setObject(image, forKey: key as NSString)
    return image
  }

  private static let rendered = NSCache<NSString, UIImage>()

  static func parse(_ data: String) -> Path {
    var path = Path()
    var numbers: [CGFloat] = []
    var command: Character = "M"
    func flush() {
      switch command {
      case "M" where numbers.count == 2: path.move(to: CGPoint(x: numbers[0], y: numbers[1]))
      case "L" where numbers.count == 2: path.addLine(to: CGPoint(x: numbers[0], y: numbers[1]))
      case "C" where numbers.count == 6:
        path.addCurve(
          to: CGPoint(x: numbers[4], y: numbers[5]),
          control1: CGPoint(x: numbers[0], y: numbers[1]),
          control2: CGPoint(x: numbers[2], y: numbers[3])
        )
      case "Z": path.closeSubpath()
      default: break
      }
      numbers = []
    }
    for token in data.split(separator: " ") {
      guard let first = token.first else { continue }
      if first.isLetter {
        flush()
        command = first
        if first == "Z" { flush() }
        let rest = token.dropFirst()
        if let value = Double(rest) { numbers.append(CGFloat(value)) }
      } else if let value = Double(token) {
        numbers.append(CGFloat(value))
      }
    }
    flush()
    return path
  }
}
