import CoreText
import SwiftUI
import UIKit

@available(iOS 26.0, *)
struct CrowbarFontModifier: ViewModifier {
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
