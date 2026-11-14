import SwiftUI

extension Color {
  /// A colour as CSS gives a theme token: `#rgb[a]`, `#rrggbb[aa]`, `rgb[a]()`,
  /// `color(srgb|display-p3 …)` or `oklch()`; nil for anything else.
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

  private static func function(_ name: String, _ args: CSSArgs) -> Color? {
    switch name {
    case "rgb", "rgba":
      guard let r = args.channel(0, scale: 255), let g = args.channel(1, scale: 255),
        let b = args.channel(2, scale: 255)
      else { return nil }
      return Color(.sRGB, red: r, green: g, blue: b, opacity: args.alpha)
    case "color":
      let space: RGBColorSpace? =
        switch args.parts.first {
        case "srgb": .sRGB
        case "display-p3": .displayP3
        default: nil
        }
      guard let space, let r = args.channel(1), let g = args.channel(2), let b = args.channel(3)
      else { return nil }
      return Color(space, red: r, green: g, blue: b, opacity: args.alpha)
    case "oklch":
      guard let l = args.channel(0), let c = args.channel(1, percentOf: 0.4) else { return nil }
      return oklch(l, c, args.number(2) ?? 0, args.alpha)
    default:
      return nil
    }
  }

  private static func hex(_ digits: Substring) -> Color? {
    let full =
      digits.count <= 4 ? digits.map { "\($0)\($0)" }.joined() : String(digits)
    guard full.count == 6 || full.count == 8, let value = UInt64(full, radix: 16) else {
      return nil
    }
    let rgba = full.count == 6 ? value << 8 | 0xFF : value
    let byte = { (shift: UInt64) in Double((rgba >> shift) & 0xFF) / 255 }
    return Color(.sRGB, red: byte(24), green: byte(16), blue: byte(8), opacity: byte(0))
  }

  /// OKLCH to linear sRGB, by the matrices of the OKLab definition.
  private static func oklch(_ l: Double, _ c: Double, _ hue: Double, _ alpha: Double) -> Color {
    let a = c * cos(hue * .pi / 180)
    let b = c * sin(hue * .pi / 180)
    let lms = [
      l + 0.3963377774 * a + 0.2158037573 * b,
      l - 0.1055613458 * a - 0.0638541728 * b,
      l - 0.0894841775 * a - 1.2914855480 * b,
    ].map { $0 * $0 * $0 }
    return Color(
      .sRGBLinear,
      red: 4.0767416621 * lms[0] - 3.3077115913 * lms[1] + 0.2309699292 * lms[2],
      green: -1.2684380046 * lms[0] + 2.6097574011 * lms[1] - 0.3413193965 * lms[2],
      blue: -0.0041960863 * lms[0] - 0.7034186147 * lms[1] + 1.7076147010 * lms[2],
      opacity: alpha)
  }
}

/// A CSS colour function's arguments, in either the comma or the space-and-slash syntax.
private struct CSSArgs {
  let parts: [String]
  let alpha: Double

  init(_ text: Substring) {
    let halves = text.split(separator: "/", maxSplits: 1)
    var parts = halves.first.map {
      $0.split(whereSeparator: { $0 == "," || $0 == " " }).map(String.init)
    } ?? []
    var alpha = halves.count == 2 ? String(halves[1]).trimmingCharacters(in: .whitespaces) : nil
    // rgba(r, g, b, a): the alpha is the fourth part.
    if alpha == nil, parts.count == 4, parts.first.flatMap(Double.init) != nil {
      alpha = parts.removeLast()
    }
    self.parts = parts
    self.alpha = alpha.flatMap { Self.value($0) } ?? 1
  }

  /// A part as CSS reads it: a number over `scale`, or a percentage of `percentOf`.
  func channel(_ index: Int, scale: Double = 1, percentOf: Double = 1) -> Double? {
    guard index < parts.count else { return nil }
    return Self.value(parts[index], scale: scale, percentOf: percentOf)
  }

  /// A plain number, such as a hue in degrees.
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
