import CoreText
import SwiftUI
import UIKit

@_silgen_name("crowbar_chrome_font")
private func crowbarChromeFont(
  _ index: Int,
  _ length: UnsafeMutablePointer<Int>
) -> UnsafePointer<UInt8>?

enum CrowbarFont {
  static let uiFontName = "CalSansUI-Regular"
  static let heading = "InstrumentSerif-Regular"

  private static let bundledFontCount = 2

  private static let registered: Bool = {
    (0..<Self.bundledFontCount)
      .map { index in
        var length = 0
        guard let bytes = crowbarChromeFont(index, &length),
          let provider = CGDataProvider(
            dataInfo: nil,
            data: bytes,
            size: length,
            releaseData: { _, _, _ in }
          ),
          let font = CGFont(provider)
        else { return false }
        return CTFontManagerRegisterGraphicsFont(font, nil)
      }
      .allSatisfy { $0 }
  }()

  private static let weightAxis = 0x7767_6874

  static func uiFont(
    size: CGFloat,
    weight: UIFont.Weight
  ) -> UIFont {
    _ = registered
    let wght = Self.wght(weight)
    var attributes: [UIFontDescriptor.AttributeName: Any] = [.name: uiFontName]
    attributes[kCTFontVariationAttribute as UIFontDescriptor.AttributeName] = [weightAxis: wght]
    let descriptor = UIFontDescriptor(fontAttributes: attributes)
    let font = UIFont(descriptor: descriptor, size: size)
    return font.familyName == "Cal Sans UI" ? font : .systemFont(ofSize: size, weight: weight)
  }

  static func headingFont(size: CGFloat) -> UIFont {
    _ = registered
    return UIFont(name: heading, size: size)
      ?? UIFont.systemFont(ofSize: size).fontDescriptor.withDesign(.serif)
      .map {
        UIFont(descriptor: $0, size: size)
      }
      ?? .systemFont(ofSize: size)
  }

  private static func wght(_ weight: UIFont.Weight) -> CGFloat {
    switch weight {
    case ..<UIFont.Weight.medium: Metric.regularWght
    case ..<UIFont.Weight.semibold: Metric.mediumWght
    case ..<UIFont.Weight.bold: Metric.semiboldWght
    default: Metric.boldWght
    }
  }
}
