import SwiftUI
import UIKit

/// One row of the Settings list: `hosts` is drawn natively, any other id is a web tab the sheet
/// hosts. `icon` is the web's name for a Phosphor icon.
struct SettingsPageItem: Decodable, Identifiable, Equatable {
  let id: String
  let label: String
  let icon: String
}

struct ThemeChoice: Decodable, Identifiable, Equatable {
  let id: String
  let name: String
}

/// What the Settings sheet shows besides the palette: the app's theme mode (`set_chrome`'s
/// `appearance`), its colour theme and pages, and the hosts the web last pushed.
final class SettingsModel: ObservableObject {
  @Published var state: SettingsState?
  @Published var themeMode = "system"
  @Published var hosts = HostListState.empty
  var send: (ChromeIntent) -> Void = { _ in }
  /// Tells the hosted page which web tab to draw; nil blanks it.
  var showTab: (String?) -> Void = { _ in }

  func applyHosts(_ snapshot: String) {
    guard let data = snapshot.data(using: .utf8),
      let list = try? JSONDecoder().decode(HostListState.self, from: data)
    else { return }
    hosts = list
  }
}

/// A screen the Settings list pushes.
enum SettingsRoute: Hashable {
  case hosts, theme
  /// A web tab, hosted.
  case page(id: String, label: String)
}
