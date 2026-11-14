import Foundation

struct PlusMenuState: Decodable, Equatable {
  let open: Bool
  let title: String
  let items: [PlusMenuItem]

  var groups: [[PlusMenuItem]] {
    items.reduce(into: [[PlusMenuItem]]()) { groups, item in
      if item.divider || groups.isEmpty {
        groups.append([item])
      } else {
        groups[groups.count - 1].append(item)
      }
    }
  }
}
