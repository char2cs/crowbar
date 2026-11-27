import Foundation

struct SettingsPageItem: Decodable, Identifiable, Equatable {
  let id: String
  let label: String
  let icon: String
}
