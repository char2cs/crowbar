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
    let image = UIGraphicsImageRenderer(size: CGSize(width: size, height: size))
      .image { _ in
        UIColor.black.setFill()
        UIColor.black.setStroke()
        paint(UIBezierPath(cgPath: path(in: size).cgPath), size: size)
      }
      .withRenderingMode(.alwaysTemplate)
    Self.rendered.setObject(image, forKey: key as NSString)
    return image
  }

  private func paint(
    _ outline: UIBezierPath,
    size: CGFloat
  ) {
    guard art.stroke > 0 else { return outline.fill() }
    outline.lineWidth = art.stroke * size / art.viewBox
    outline.lineCapStyle = .round
    outline.lineJoinStyle = .round
    outline.stroke()
  }

  private static let rendered = NSCache<NSString, UIImage>()

  static func parse(_ data: String) -> Path {
    var parser = PathParser()
    for token in data.split(separator: " ") { parser.feed(token) }
    parser.flush()
    return parser.path
  }
}
