import Foundation

enum Reveal: String, Decodable {
  case none, chatList, ide, files
}

extension Reveal {
  var screen: Screen {
    switch self {
    case .none: return .chat
    case .chatList: return .chatList
    case .ide: return .ide
    case .files: return .files
    }
  }
}
