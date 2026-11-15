import Foundation

struct PlusMenuState: Decodable, Equatable {
  let open: Bool
  let title: String
  let items: [PlusMenuItem]

  var groups: [[PlusMenuItem]] {
    items.reduce(into: [[PlusMenuItem]]()) { groups, item in
      let startsGroup = item.divider || groups.isEmpty
      let next = startsGroup ? [item] : groups.removeLast() + [item]
      groups.append(next)
    }
  }
}
