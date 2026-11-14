import CoreText
import SwiftUI
import UIKit

@available(iOS 26.0, *)
struct CrowbarHeadingModifier: ViewModifier {
  @ScaledMetric private var size: CGFloat

  init(size: CGFloat, style: Font.TextStyle) {
    _size = ScaledMetric(wrappedValue: size, relativeTo: style)
  }

  func body(content: Content) -> some View {
    content.font(Font(CrowbarFont.headingFont(size: size)))
  }
}
