import Foundation

struct TabsState: Decodable, Equatable {
  struct Tab: Decodable, Identifiable, Equatable {
    let id: String
    let title: String
    let kind: String
    let path: String?
    let active: Bool
    let dirty: Bool
    let closable: Bool
  }
  struct NewTabAction: Decodable, Identifiable, Equatable {
    let id: String
    let title: String
    let icon: String
  }
  let tabs: [Tab]
  let newTabActions: [NewTabAction]
}
