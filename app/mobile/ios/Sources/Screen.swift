import Foundation

enum Screen: Int, Comparable {
  case chatList = 0, chat, ide, files

  static func < (a: Screen, b: Screen) -> Bool { a.rawValue < b.rawValue }
}
