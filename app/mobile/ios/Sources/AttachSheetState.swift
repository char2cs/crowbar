import Foundation

struct AttachSheetState: Decodable, Equatable {
  let open: Bool
  let title: String
  let note: String
  let sources: [AttachSource]
  let files: [AttachFile]
  let hint: String?
  let attachLabel: String
  let canAttach: Bool
}
