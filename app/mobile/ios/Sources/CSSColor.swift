import SwiftUI

extension Color {
  init?(css raw: String) {
    guard let color = CSSColor.parse(raw) else { return nil }
    self = color
  }
}

enum CSSColor {
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
  private static let redIndex = 0
  private static let greenIndex = 1
  private static let blueIndex = 2
  private static let hueIndex = 2

  static func parse(_ raw: String) -> Color? {
    let css = raw.trimmingCharacters(in: .whitespaces).lowercased()
    if css.hasPrefix("#") { return hex(css.dropFirst()) }
    guard let open = css.firstIndex(of: "("), css.hasSuffix(")") else { return nil }
    let name = css[..<open]
    let args = CSSArgs(css[css.index(after: open)..<css.index(before: css.endIndex)])
    return function(String(name), args)
  }

  private static func function(
    _ name: String,
    _ args: CSSArgs
  ) -> Color? {
    switch name {
    case "rgb", "rgba": rgb(args)
    case "color": predefined(args)
    case "oklch": oklch(args)
    default: nil
    }
  }

  private static func rgb(_ args: CSSArgs) -> Color? {
    guard let red = args.channel(0, scale: Self.byteMax),
      let green = args.channel(1, scale: Self.byteMax),
      let blue = args.channel(Self.blueIndex, scale: Self.byteMax)
    else { return nil }
    return Color(.sRGB, red: red, green: green, blue: blue, opacity: args.alpha)
  }

  private static func predefined(_ args: CSSArgs) -> Color? {
    let space: Color.RGBColorSpace? =
      switch args.parts.first {
      case "srgb": .sRGB
      case "display-p3": .displayP3
      default: nil
      }
    let channels = (1...Self.rgbChannelCount).compactMap { args.channel($0) }
    guard let space, channels.count == Self.rgbChannelCount else { return nil }
    return color(space, channels, alpha: args.alpha)
  }

  private static func oklch(_ args: CSSArgs) -> Color? {
    guard let lightness = args.channel(0),
      let chroma = args.channel(1, percentOf: Self.fullChroma)
    else { return nil }
    let hue = args.number(Self.hueIndex) ?? 0
    let radians = hue * .pi / Self.halfTurnDegrees
    let lab = [lightness, chroma * cos(radians), chroma * sin(radians)]
    let lms = OKLabMatrix.apply(OKLabMatrix.lmsFromLab, to: lab).map { $0 * $0 * $0 }
    let rgb = OKLabMatrix.apply(OKLabMatrix.linearRGBFromLms, to: lms)
    return color(.sRGBLinear, rgb, alpha: args.alpha)
  }

  private static func color(
    _ space: Color.RGBColorSpace,
    _ channels: [Double],
    alpha: Double
  ) -> Color {
    Color(
      space,
      red: channels[Self.redIndex],
      green: channels[Self.greenIndex],
      blue: channels[Self.blueIndex],
      opacity: alpha
    )
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
}
