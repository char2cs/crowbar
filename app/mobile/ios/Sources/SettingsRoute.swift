import Foundation

enum SettingsRoute: Hashable {
  case hosts, theme
  case page(id: String, label: String)
}
