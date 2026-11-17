import CoreText
import SwiftUI
import UIKit

@available(iOS 26.0, *)
struct CrowbarFontModifier: ViewModifier {
  @ScaledMetric private var size: CGFloat
  let weight: Font.Weight

  init(
    style: Font.TextStyle,
    weight: Font.Weight
  ) {
    _size = ScaledMetric(wrappedValue: Self.baseSize(style), relativeTo: style)
    self.weight = weight
  }

  func body(content: Content) -> some View {
    content.font(Font(CrowbarFont.uiFont(size: size, weight: Self.uiWeight(weight))))
  }

  private static func baseSize(_ style: Font.TextStyle) -> CGFloat {
    switch style {
    case .largeTitle: Metric.largeTitleSize
    case .title: Metric.title1Size
    case .title2: Metric.title2Size
    case .title3: Metric.title3Size
    case .headline, .body: Metric.bodySize
    case .callout: Metric.calloutSize
    case .subheadline: Metric.subheadlineSize
    case .footnote: Metric.footnoteSize
    case .caption: Metric.captionSize
    default: Metric.fallbackTextSize
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
