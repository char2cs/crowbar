import Foundation

struct HostListState: Decodable, Equatable {
  let hosts: [PickerHost]
  let revoking: [String]
  let shownHost: String?

  static let empty = HostListState(hosts: [], revoking: [], shownHost: nil)
}
