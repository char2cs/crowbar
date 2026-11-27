import SwiftUI
import UIKit

final class SettingsModel: ObservableObject {
  @Published var state: SettingsState?
  @Published var themeMode = "system"
  @Published var hosts = HostListState.empty
  var send: (ChromeIntent) -> Void = { _ in }
  var showTab: (String?) -> Void = { _ in }

  func applyHosts(_ snapshot: String) {
    guard let data = snapshot.data(using: .utf8),
      let list = try? JSONDecoder().decode(HostListState.self, from: data)
    else { return }
    hosts = list
  }
}
