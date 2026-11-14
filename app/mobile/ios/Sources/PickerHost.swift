import Foundation

struct PickerHost: Decodable, Identifiable, Equatable {
  let id: String
  let name: String
  let endpoint: String
  let state: String
}
