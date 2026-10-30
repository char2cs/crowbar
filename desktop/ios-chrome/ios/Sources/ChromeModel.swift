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
  let icon: String
  let side: String
  let label: String
  let enabled: Bool
  /// Sent as `{ type: intent }` instead of a tap.
  let intent: String?
}

/// Each screen's own header buttons; both always come, so a header stays with its screen.
struct ChromeHeaders: Decodable, Equatable {
  let chat: [ChromeButton]
  let ide: [ChromeButton]
}

/// The whole view-model `set_chrome` hands over; a part it leaves out is hidden.
struct ChromeState: Decodable {
  let accent: String?
  let brandGround: String?
  let welcome: WelcomeState?
  let connect: ConnectState?
  let headers: ChromeHeaders?
  let reveal: Reveal?
  /// The newest navigation intent (its `seq`) this state already reflects.
  let ack: Int?
  let settings: SettingsState?
  let appearance: String?
  /// Where the shown host runs: `local` or `remote`.
  let hostPlace: String?
  /// The shown host's status, as the web's `HostDotState`.
  let hostStatus: String?
  let palette: PaletteState?
}

/// The theme's colours for the native sheets, as CSS gives them.
struct PaletteState: Decodable {
  let surface: String
  let raised: String
  let text: String
  let secondaryText: String
  /// The brand in each mode, whichever is on; nil from an older web.
  let brandLight: String?
  let brandDark: String?
  /// The chat list's foot over the dark chrome sidebar; nil where the sidebar keeps the plain chrome.
  let sidebar: SidebarPaletteState?
}

struct SidebarPaletteState: Decodable {
  let text: String
  let brand: String
  let brandText: String
}

/// The sidebar's colours: light text on the dark chrome, and the prominent button's fill with its label.
struct SidebarPalette: Equatable {
  let text: Color
  let brand: Color
  let brandText: Color

  init?(_ state: SidebarPaletteState) {
    guard let text = Color(css: state.text), let brand = Color(css: state.brand),
      let brandText = Color(css: state.brandText)
    else { return nil }
    self.text = text
    self.brand = brand
    self.brandText = brandText
  }
}

/// The web's palette, decoded: a sheet painted from it reads as one of the web's pages.
struct ChromePalette: Equatable {
  let surface: Color
  /// Cards and fields, one step lifted from `surface`.
  let raised: Color
  let text: Color
  let secondaryText: Color
  /// The brand in light and in dark mode, for previews of each.
  let brandLight: Color?
  let brandDark: Color?
  let sidebar: SidebarPalette?

  /// The brand in the mode the sheet is drawn in.
  func brand(_ scheme: ColorScheme) -> Color? { scheme == .dark ? brandDark : brandLight }

  init?(_ state: PaletteState) {
    guard let surface = Color(css: state.surface), let raised = Color(css: state.raised),
      let text = Color(css: state.text), let secondaryText = Color(css: state.secondaryText)
    else { return nil }
    self.surface = surface
    self.raised = raised
    self.text = text
    self.secondaryText = secondaryText
    brandLight = state.brandLight.flatMap { Color(css: $0) }
    brandDark = state.brandDark.flatMap { Color(css: $0) }
    sidebar = state.sidebar.flatMap(SidebarPalette.init)
  }
}

/// The Settings sheet: a native list of the web's pages, and the choices drawn natively.
struct SettingsState: Decodable, Equatable {
  let open: Bool
  let theme: String
  let themes: [ThemeChoice]
  let pages: [SettingsPageItem]
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
  @Published var hostPlace = "remote"
  @Published var hostStatus = "unknown"
  @Published var welcome: WelcomeState?
  // Kept after the web closes the sheet, so it slides away showing what it showed.
  @Published var connect: ConnectState?
  @Published var headers = ChromeHeaders(chat: [], ide: [])
  @Published var context: ContextState?
  var send: (ChromeIntent) -> Void = { _ in }
  /// Shows a native dialog over whatever is up.
  var present: (UIViewController) -> Void = { _ in }
}
