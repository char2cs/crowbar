import Foundation

@testable import tauri_plugin_crowbar_chrome

struct FakeWeb {
  var screen = Screen.chat
  var ack = 0
  var inbox: [(String, Int)] = []

  mutating func receive(_ commands: [NavigationCommand]) {
    for case .send(let name, let seq) in commands { inbox.append((name, seq)) }
  }

  mutating func apply(
    _ count: Int = .max
  ) -> (
    Screen,
    Int
  ) {
    for (name, seq) in inbox.prefix(count) {
      switch name {
      case "openChatList": screen = .chatList
      case "closeChatList": screen = .chat
      case "openIde": screen = .ide
      case "closeIde": screen = .chat
      case "openFiles": screen = .files
      default: screen = .ide
      }
      ack = seq
    }
    inbox.removeFirst(min(count, inbox.count))
    return (screen, ack)
  }
}
