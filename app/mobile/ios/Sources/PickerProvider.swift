import Foundation

struct PickerProvider: Decodable, Identifiable, Equatable {
  let id: String
  let name: String
  let icon: String?
  let fallbackIcon: String
  let models: [PickerModel]
}
