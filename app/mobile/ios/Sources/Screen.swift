import Foundation

enum Screen: Int, Comparable {
  case chatList = 0, chat, ide, files

  static func < (lhs: Screen, rhs: Screen) -> Bool { lhs.rawValue < rhs.rawValue }
}
