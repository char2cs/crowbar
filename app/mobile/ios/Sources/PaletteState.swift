import Foundation

struct PaletteState: Decodable {
  let surface: String
  let raised: String
  let text: String
  let secondaryText: String
  let brandLight: String?
  let brandDark: String?
  let sidebar: SidebarPaletteState?
}
