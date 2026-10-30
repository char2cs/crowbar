import CoreText
import SwiftUI
import UIKit

@_silgen_name("crowbar_chrome_font")
private func crowbarChromeFont(_ index: Int, _ length: UnsafeMutablePointer<Int>)
  -> UnsafeRawPointer?

/// The fonts the web ships, carried in the plugin's Rust half: the UI font (`CalSansUI`,
/// `--font-sans`) and the heading font (`Instrument Serif`, `--font-heading`). The UI font's
/// weight axis runs 400–700, so Regular to Bold are real faces; the heading font has one weight.
enum CrowbarFont {
  static let ui = "CalSansUI-Regular"
  static let heading = "InstrumentSerif-Regular"

  /// Both faces, registered once for the process.
  private static let registered: Bool = {
    (0..<2).allSatisfy { index in
      var length = 0
      guard let bytes = crowbarChromeFont(index, &length),
        let provider = CGDataProvider(
          dataInfo: nil, data: bytes, size: length, releaseData: { _, _, _ in }),
        let font = CGFont(provider)
      else { return false }
      return CTFontManagerRegisterGraphicsFont(font, nil)
    }
  }()

  private static let weightAxis = 0x7767_6874  // 'wght'

  /// The face at `size` and `weight`, or the system font where the file did not load.
  static func uiFont(size: CGFloat, weight: UIFont.Weight) -> UIFont {
    _ = registered
    let wght = Self.wght(weight)
    let descriptor = UIFontDescriptor(fontAttributes: [
      .name: ui,
      kCTFontVariationAttribute as UIFontDescriptor.AttributeName: [weightAxis: wght],
    ])
    let font = UIFont(descriptor: descriptor, size: size)
    return font.familyName == "Cal Sans UI" ? font : .systemFont(ofSize: size, weight: weight)
  }

  /// The heading face at `size`, or the system's serif where it did not load.
  static func headingFont(size: CGFloat) -> UIFont {
    _ = registered
    return UIFont(name: heading, size: size)
      ?? UIFont(descriptor: UIFont.systemFont(ofSize: size).fontDescriptor.withDesign(.serif)!, size: size)
  }

  private static func wght(_ weight: UIFont.Weight) -> CGFloat {
    switch weight {
    case ..<UIFont.Weight.medium: 400
    case ..<UIFont.Weight.semibold: 500
    case ..<UIFont.Weight.bold: 600
    default: 700
    }
  }
}

/// Applies the UI font at a text style's size, scaling with Dynamic Type.
@available(iOS 26.0, *)
private struct CrowbarFontModifier: ViewModifier {
  @ScaledMetric private var size: CGFloat
  let weight: Font.Weight

  init(style: Font.TextStyle, weight: Font.Weight) {
    _size = ScaledMetric(wrappedValue: Self.baseSize(style), relativeTo: style)
    self.weight = weight
  }

  func body(content: Content) -> some View {
    content.font(Font(CrowbarFont.uiFont(size: size, weight: Self.uiWeight(weight))))
  }

  private static func baseSize(_ style: Font.TextStyle) -> CGFloat {
    switch style {
    case .largeTitle: 34
    case .title: 28
    case .title2: 22
    case .title3: 20
    case .headline, .body: 17
    case .callout: 16
    case .subheadline: 15
    case .footnote: 13
    case .caption: 12
    default: 11
    }
  }

  private static func uiWeight(_ weight: Font.Weight) -> UIFont.Weight {
    switch weight {
    case .medium: .medium
    case .semibold: .semibold
    case .bold, .heavy, .black: .bold
    default: .regular
    }
  }
}

/// Applies the heading font at `size`, scaling with Dynamic Type relative to `style`.
@available(iOS 26.0, *)
private struct CrowbarHeadingModifier: ViewModifier {
  @ScaledMetric private var size: CGFloat

  init(size: CGFloat, style: Font.TextStyle) {
    _size = ScaledMetric(wrappedValue: size, relativeTo: style)
  }

  func body(content: Content) -> some View {
    content.font(Font(CrowbarFont.headingFont(size: size)))
  }
}

extension View {
  /// The app's heading font (section headers, navigation titles).
  @available(iOS 26.0, *)
  func crowbarHeading(size: CGFloat, relativeTo style: Font.TextStyle) -> some View {
    modifier(CrowbarHeadingModifier(size: size, style: style))
  }

  /// The app's UI font in place of the system's, at `style`'s size.
  @available(iOS 26.0, *)
  func crowbarUI(_ style: Font.TextStyle = .body, weight: Font.Weight = .regular) -> some View {
    modifier(CrowbarFontModifier(style: style, weight: weight))
  }

  /// A native grouped list in the web palette and UI font: the Settings and host screens' base.
  @available(iOS 26.0, *)
  func nativeList(_ palette: ChromePalette?) -> some View {
    listStyle(.insetGrouped)
      .scrollContentBackground(palette == nil ? .automatic : .hidden)
      .background(palette?.surface.ignoresSafeArea())
      .crowbarUI(.body)
  }

  /// An inline title in the UI font (the system's navigation title takes no custom font).
  @available(iOS 26.0, *)
  func nativeTitle(_ title: String) -> some View {
    navigationTitle(title)
      .navigationBarTitleDisplayMode(.inline)
      .toolbar {
        ToolbarItem(placement: .principal) {
          Text(title).crowbarHeading(size: 24, relativeTo: .title2)
        }
      }
  }
}
