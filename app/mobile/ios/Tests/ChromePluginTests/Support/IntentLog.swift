@testable import tauri_plugin_crowbar_chrome

final class IntentLog {
  private(set) var sent: [ChromeIntent] = []

  var types: [String] { sent.compactMap { $0.payload["type"] as? String } }

  func record(_ intent: ChromeIntent) {
    sent.append(intent)
  }
}
