import Foundation

struct ChromeState: Decodable {
  let accent: String?
  let brandGround: String?
  let welcome: WelcomeState?
  let connect: ConnectState?
  let headers: ChromeHeaders?
  let reveal: Reveal?
  let ack: Int?
  let settings: SettingsState?
  let appearance: String?
  let hostPlace: String?
  let hostStatus: String?
  let palette: PaletteState?
}
