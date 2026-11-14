import Foundation

struct PickerPermission: Decodable, Equatable {
  let levels: [PickerPermissionLevel]
  let level: String
}
