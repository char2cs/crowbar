import SwiftUI

enum HostDot {
  enum Tone { case online, offline, needsAttention, unsettled }

  static func tone(_ state: String) -> Tone {
    switch state {
    case "up", "no-live-stream": .online
    case "down", "not-available": .offline
    case "needs-repair": .needsAttention
    default: .unsettled
    }
  }

  static func color(_ tone: Tone) -> Color {
    switch tone {
    case .online: .green
    case .offline: .red
    case .needsAttention: .orange
    case .unsettled: .gray
    }
  }

  static func words(_ state: String) -> String {
    switch state {
    case "up": "Online"
    case "down": "Offline"
    case "needs-repair": "Needs pairing again"
    case "no-live-stream": "Online, no live stream"
    case "not-available": "Host not available"
    case "probing": "Checking"
    default: "Status unknown"
    }
  }
}
