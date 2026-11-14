import SwiftUI
import Tauri

final class ChromeModel: ObservableObject {
  @Published var accent: Color?
  @Published var brandGround: Color?
  @Published var palette: ChromePalette?
  @Published var hostPlace = "remote"
  @Published var hostStatus = "unknown"
  @Published var welcome: WelcomeState?
  @Published var jumpLabel = ""
  var jumpOwner = ""
  @Published var connect: ConnectState?
  @Published var headers = ChromeHeaders(chat: [], ide: [])
  var send: (ChromeIntent) -> Void = { _ in }
  var present: (UIViewController) -> Void = { _ in }
}
