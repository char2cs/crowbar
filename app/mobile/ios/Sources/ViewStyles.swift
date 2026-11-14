import CoreText
import SwiftUI
import UIKit

extension View {
  @available(iOS 26.0, *)
  func crowbarHeading(size: CGFloat, relativeTo style: Font.TextStyle) -> some View {
    modifier(CrowbarHeadingModifier(size: size, style: style))
  }

  @available(iOS 26.0, *)
  func crowbarUI(_ style: Font.TextStyle = .body, weight: Font.Weight = .regular) -> some View {
    modifier(CrowbarFontModifier(style: style, weight: weight))
  }

  @available(iOS 26.0, *)
  func nativeList(_ palette: ChromePalette?) -> some View {
    listStyle(.insetGrouped)
      .scrollContentBackground(palette == nil ? .automatic : .hidden)
      .background(palette?.surface.ignoresSafeArea())
      .crowbarUI(.body)
  }

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
