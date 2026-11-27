import Foundation

struct PickerPermissionLevel: Decodable, Identifiable, Equatable {
  let value: String
  let label: String
  let description: String
  var id: String { value }
}
