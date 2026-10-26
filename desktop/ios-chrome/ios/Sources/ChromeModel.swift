import SwiftUI
import Tauri

struct WelcomeState: Decodable, Equatable {
  let visible: Bool
  let label: String
}

enum ConnectStage: String, Decodable {
  case address, fingerprint, code
}

struct ConnectState: Decodable, Equatable {
  let open: Bool
  let stage: ConnectStage
  let name: String
  let address: String
  let code: String
  let fingerprint: String
  let busy: Bool
  let error: String?
}

struct ChromeButton: Decodable, Identifiable, Equatable {
  let id: String
  let symbol: String
  let side: String
  let label: String
  let enabled: Bool
  /// Sent as `{ type: intent }` instead of a tap.
  let intent: String?
}

/// The whole view-model `set_chrome` hands over; a part it leaves out is hidden.
struct ChromeState: Decodable {
  let accent: String?
  let brandGround: String?
  let welcome: WelcomeState?
  let connect: ConnectState?
  let buttons: [ChromeButton]?
  let reveal: Reveal?
  let settings: SettingsState?
  let appearance: String?
  let palette: PaletteState?
}

/// The theme's colours for the native sheets, as CSS gives them.
struct PaletteState: Decodable {
  let surface: String
  let raised: String
  let text: String
  let secondaryText: String
}

/// The web's palette, decoded: a sheet painted from it reads as one of the web's pages.
struct ChromePalette: Equatable {
  let surface: Color
  /// Cards and fields, one step lifted from `surface`.
  let raised: Color
  let text: Color
  let secondaryText: Color

  init?(_ state: PaletteState) {
    guard let surface = Color(css: state.surface), let raised = Color(css: state.raised),
      let text = Color(css: state.text), let secondaryText = Color(css: state.secondaryText)
    else { return nil }
    self.surface = surface
    self.raised = raised
    self.text = text
    self.secondaryText = secondaryText
  }
}

/// The Settings sheet; its one page is the web's own settings.
struct SettingsState: Decodable, Equatable {
  let open: Bool
}

/// Which layer the front card is slid aside to show.
enum Reveal: String, Decodable {
  case none, chatList, ide, files
}

/// What the user asked for, reported to the web as `{ type, ...fields }`; the web decides.
struct ChromeIntent {
  let payload: JSObject

  static func of(_ type: String, _ fields: JSObject = [:]) -> ChromeIntent {
    var payload = fields
    payload["type"] = type
    return ChromeIntent(payload: payload)
  }

  static var start: ChromeIntent { of("start") }
  static var dismiss: ChromeIntent { of("dismiss") }
  static var back: ChromeIntent { of("back") }
  static var confirmFingerprint: ChromeIntent { of("confirmFingerprint") }
  static func submitAddress(name: String, address: String) -> ChromeIntent {
    of("submitAddress", ["name": name, "address": address])
  }
  static func submitCode(_ code: String) -> ChromeIntent { of("submitCode", ["code": code]) }
  static func tap(_ id: String) -> ChromeIntent { of("tap", ["id": id]) }
}

/// What the web last handed us; SwiftUI re-renders from it and nothing else.
final class ChromeModel: ObservableObject {
  @Published var accent: Color?
  @Published var brandGround: Color?
  @Published var palette: ChromePalette?
  @Published var welcome: WelcomeState?
  // Kept after the web closes the sheet, so it slides away showing what it showed.
  @Published var connect: ConnectState?
  @Published var buttons: [ChromeButton] = []
  @Published var context: ContextState?
  var send: (ChromeIntent) -> Void = { _ in }
  /// Shows a native dialog over whatever is up.
  var present: (UIViewController) -> Void = { _ in }
}
