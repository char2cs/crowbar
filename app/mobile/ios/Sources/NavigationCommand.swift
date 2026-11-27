import Foundation

enum NavigationCommand: Equatable {
  case send(String, seq: Int)
  case freezeChat
  case dropSnapshot
}
