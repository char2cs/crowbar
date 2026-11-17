import SwiftUI

extension Color {
  private static let byteMax = 255.0
  private static let byteMask: UInt64 = 0xFF
  private static let halfTurnDegrees = 180.0
  private static let fullChroma = 0.4
  private static let hexRadix = 16
  private static let shortHexMax = 4
  private static let alphaBits: UInt64 = 8
  private static let rgbChannelCount = 3
  private static let rgbDigits = 6
  private static let rgbaDigits = 8
  private static let redShift: UInt64 = 24
  private static let greenShift: UInt64 = 16
  private static let blueShift: UInt64 = 8
  private static let alphaShift: UInt64 = 0

  init?(css raw: String) {
    let css = raw.trimmingCharacters(in: .whitespaces).lowercased()
    if css.hasPrefix("#") {
      guard let color = Self.hex(css.dropFirst()) else { return nil }
      self = color
      return
    }
    guard let open = css.firstIndex(of: "("), css.hasSuffix(")") else { return nil }
    let name = css[..<open]
    let args = CSSArgs(css[css.index(after: open)..<css.index(before: css.endIndex)])
    guard let color = Self.function(String(name), args) else { return nil }
    self = color
  }

  private static func function(
    _ name: String,
    _ args: CSSArgs
  ) -> Color? {
    switch name {
    case "rgb", "rgba":
      guard let r = args.channel(0, scale: Self.byteMax),
        let g = args.channel(1, scale: Self.byteMax),
        let b = args.channel(2, scale: Self.byteMax)
      else { return nil }
      return Color(.sRGB, red: r, green: g, blue: b, opacity: args.alpha)
    case "color":
      let space: RGBColorSpace? =
        switch args.parts.first {
        case "srgb": .sRGB
        case "display-p3": .displayP3
        default: nil
        }
      let channels = (1...Self.rgbChannelCount).compactMap { args.channel($0) }
      guard let space, channels.count == Self.rgbChannelCount else { return nil }
      return Color(
        space,
        red: channels[0],
        green: channels[1],
        blue: channels[2],
        opacity: args.alpha
      )
    case "oklch":
      guard let l = args.channel(0), let c = args.channel(1, percentOf: Self.fullChroma) else {
        return nil
      }
      return oklch(l, c, args.number(2) ?? 0, args.alpha)
    default:
      return nil
    }
  }

  private static func hex(_ digits: Substring) -> Color? {
    let full =
      digits.count <= Self.shortHexMax ? digits.map { "\($0)\($0)" }.joined() : String(digits)
    guard full.count == Self.rgbDigits || full.count == Self.rgbaDigits,
      let value = UInt64(full, radix: Self.hexRadix)
    else {
      return nil
    }
    let rgba = full.count == Self.rgbDigits ? value << Self.alphaBits | Self.byteMask : value
    let byte = { (shift: UInt64) in Double((rgba >> shift) & Self.byteMask) / Self.byteMax }
    return Color(
      .sRGB,
      red: byte(Self.redShift),
      green: byte(Self.greenShift),
      blue: byte(Self.blueShift),
      opacity: byte(Self.alphaShift)
    )
  }

  private static func oklch(
    _ l: Double,
    _ c: Double,
    _ hue: Double,
    _ alpha: Double
  ) -> Color {
    let radians = hue * .pi / Self.halfTurnDegrees
    let lms =
      OKLabMatrix.apply(OKLabMatrix.lmsFromLab, to: [l, c * cos(radians), c * sin(radians)])
      .map { $0 * $0 * $0 }
    let rgb = OKLabMatrix.apply(OKLabMatrix.linearRGBFromLms, to: lms)
    return Color(
      .sRGBLinear,
      red: rgb[0],
      green: rgb[1],
      blue: rgb[2],
      opacity: alpha
    )
  }
}
