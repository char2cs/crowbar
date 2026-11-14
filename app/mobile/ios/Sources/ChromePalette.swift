import SwiftUI

struct ChromePalette: Equatable {
  let surface: Color
  let raised: Color
  let text: Color
  let secondaryText: Color
  let brandLight: Color?
  let brandDark: Color?
  let sidebar: SidebarPalette?

  func brand(_ scheme: ColorScheme) -> Color? { scheme == .dark ? brandDark : brandLight }

  init?(_ state: PaletteState) {
    guard let surface = Color(css: state.surface), let raised = Color(css: state.raised),
      let text = Color(css: state.text), let secondaryText = Color(css: state.secondaryText)
    else { return nil }
    self.surface = surface
    self.raised = raised
    self.text = text
    self.secondaryText = secondaryText
    brandLight = state.brandLight.flatMap { Color(css: $0) }
    brandDark = state.brandDark.flatMap { Color(css: $0) }
    sidebar = state.sidebar.flatMap(SidebarPalette.init)
  }
}
