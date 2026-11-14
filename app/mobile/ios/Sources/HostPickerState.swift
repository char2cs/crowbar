import Foundation

struct HostPickerState: Decodable, Equatable {
  let open: Bool
  let list: HostListState

  private enum Keys: String, CodingKey { case open }

  init(open: Bool, list: HostListState) {
    self.open = open
    self.list = list
  }

  init(from decoder: Decoder) throws {
    open = try decoder.container(keyedBy: Keys.self).decode(Bool.self, forKey: .open)
    list = try HostListState(from: decoder)
  }
}
