import Foundation

struct AttachFile: Decodable, Identifiable, Equatable {
  let id: String
  let name: String
  let detail: String
  let status: String
  let percent: Int?
  let canRetry: Bool
}
