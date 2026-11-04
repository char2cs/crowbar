import SwiftUI
import UIKit

extension CrowbarIcon {
  /// The icon the web names `name`, or nil for a name this build does not draw.
  init?(named name: String) { self.init(rawValue: name) }

  /// The icon's outline in a `size`-point square: the generated path data, scaled.
  func path(in size: CGFloat) -> Path {
    let scale = size / art.viewBox
    return Self.outlines[self, default: Path()].applying(CGAffineTransform(scaleX: scale, y: scale))
  }

  /// Every icon's outline in its own viewBox, parsed once.
  private static let outlines: [CrowbarIcon: Path] = Dictionary(
    uniqueKeysWithValues: allCases.map { icon in
      var outline = Path()
      for data in icon.art.paths { outline.addPath(parse(data)) }
      return (icon, outline)
    })

  /// A template image of the icon, for the APIs that take an image (menus, swipe actions).
  func image(size: CGFloat) -> UIImage {
    let key = "\(rawValue)@\(size)"
    if let cached = Self.images.object(forKey: key as NSString) { return cached }
    let art = art
    let image = UIGraphicsImageRenderer(size: CGSize(width: size, height: size)).image { context in
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
    }.withRenderingMode(.alwaysTemplate)
    Self.images.setObject(image, forKey: key as NSString)
    return image
  }

  private static let images = NSCache<NSString, UIImage>()

  /// Parses the generator's normalised data: absolute M, L, C and Z, space separated.
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
          to: CGPoint(x: numbers[4], y: numbers[5]), control1: CGPoint(x: numbers[0], y: numbers[1]),
          control2: CGPoint(x: numbers[2], y: numbers[3]))
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

/// A Phosphor icon the way the web draws it: tinted by `foregroundStyle`, sized like a symbol
/// (it scales with Dynamic Type from `size` points at the default text size).
@available(iOS 26.0, *)
struct PhosphorIcon: View {
  let icon: CrowbarIcon
  /// Extra outline in points: the regular weight is 1.25pt at 20pt, thin on clear glass.
  let emphasis: CGFloat
  @ScaledMetric private var side: CGFloat

  init(_ icon: CrowbarIcon, size: CGFloat = 20, emphasis: CGFloat = 0) {
    self.icon = icon
    self.emphasis = emphasis
    _side = ScaledMetric(wrappedValue: size, relativeTo: .body)
  }

  var body: some View {
    let art = icon.art
    let outline = icon.path(in: side)
    Group {
      if art.stroke > 0 {
        outline.stroke(style: StrokeStyle(lineWidth: art.stroke * side / art.viewBox, lineCap: .round, lineJoin: .round))
      } else {
        outline.fill().overlay { if emphasis > 0 { outline.stroke(lineWidth: emphasis) } }
      }
    }
    .frame(width: side, height: side)
    .accessibilityHidden(true)
  }
}
