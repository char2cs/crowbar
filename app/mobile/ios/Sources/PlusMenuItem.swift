import Foundation

struct PlusMenuItem: Decodable, Identifiable, Equatable {
  let id: String
  let title: String
  let icon: String
  let hint: String?
  let divider: Bool
}
