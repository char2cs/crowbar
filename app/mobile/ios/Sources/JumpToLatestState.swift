import Foundation

struct JumpToLatestState: Decodable, Equatable {
  let owner: String
  let visible: Bool
  let bottom: Double
  let label: String
}
