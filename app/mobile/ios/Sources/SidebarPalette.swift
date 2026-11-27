import SwiftUI

struct SidebarPalette: Equatable {
  let text: Color
  let brand: Color
  let brandText: Color

  init?(_ state: SidebarPaletteState) {
    guard let text = Color(css: state.text), let brand = Color(css: state.brand),
      let brandText = Color(css: state.brandText)
    else { return nil }
    self.text = text
    self.brand = brand
    self.brandText = brandText
  }
}
