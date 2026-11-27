import Foundation

struct SettingsState: Decodable, Equatable {
  let open: Bool
  let theme: String
  let themes: [ThemeChoice]
  let pages: [SettingsPageItem]
}
