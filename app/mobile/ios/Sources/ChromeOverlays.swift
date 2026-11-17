import SwiftUI
import UIKit
import WebKit

@available(iOS 26.0, *)
final class ChromeOverlays {
  let model: ChromeModel
  private let scheme: AppScheme
  private let sidebar: PageChannel
  private let settings: PageChannel
  private let files: PageChannel
  let picker = ModelPickerModel()
  let hostPicker = HostPickerModel()
  let plus = PlusMenuModel()
  let attach = AttachSheetModel()
  let settingsModel = SettingsModel()
  let tabs = TabsModel()
  private var rowMenu: RowMenu?
  weak var webview: WKWebView?
  private var navigation: NavigationController?
  private var keyboard: KeyboardFit?
  var jumpBottom: CGFloat = 0
  private var settingsWarm: WKWebView?
  var hosted: [UIViewController] = []
  var surfaces: [NSKeyValueObservation] = []
  lazy var sheets = makeConnectSheet()
  lazy var settingsSheet = makeSettingsSheet()
  lazy var pickerSheet = makePickerSheet()
  lazy var plusSheet = makePlusSheet()
  lazy var attachSheet = makeAttachSheet()
  lazy var hostSheet = makeHostSheet()
  lazy var welcome = makeWelcome()
  lazy var tabStrip = makeTabStrip()
  lazy var jumpButton: UIView? = makeJumpButton()

  init(
    model: ChromeModel,
    webview: WKWebView,
    scheme: AppScheme,
    send: @escaping (ChromeIntent) -> Void
  ) {
    self.model = model
    self.scheme = scheme
    self.webview = webview
    sidebar = PageChannel(
      handler: "sidebar",
      feeds: ["sidebarReady": "__crowbarSidebar"],
      send: send
    )
    var settingsFeeds = ["agentsSettingsReady": "__crowbarAgentsSettings"]
    settingsFeeds["settingsTabReady"] = "__crowbarSettingsTab"
    settings = PageChannel(handler: "settings", feeds: settingsFeeds, send: send)
    files = PageChannel(handler: "files", feeds: ["filesReady": "__crowbarFiles"], send: send)
    wire(webview: webview, send: send)
  }

  private func wire(
    webview: WKWebView,
    send: @escaping (ChromeIntent) -> Void
  ) {
    picker.send = send
    hostPicker.send = send
    settingsModel.send = send
    settingsModel.showTab = { [settings] tab in
      let json = (try? JSONEncoder().encode(tab)).flatMap { String(data: $0, encoding: .utf8) }
      settings.render(json ?? "null", to: "__crowbarSettingsTab")
    }
    tabs.send = send
    model.present = { [weak webview] dialog in
      guard let root = webview?.window?.rootViewController else { return }
      Dialogs.topmost(root).present(dialog, animated: true)
    }
    if let host = webview.superview { mountNavigation(in: host, webview: webview, send: send) }
    model.send = { [weak self] intent in
      if intent.payload["id"] as? String == "ide" { self?.navigation?.willOpenIde() }
      send(intent)
    }
  }

  private func mountNavigation(
    in host: UIView,
    webview: WKWebView,
    send: @escaping (ChromeIntent) -> Void
  ) {
    let list = mountSidebar(webview: webview, send: send)
    keyboard = KeyboardFit(webview: webview)
    keyboard?.fitted = { [weak self] in self?.placeJumpButton() }
    paintSurface(under: webview, host: host)
    FormAccessory.hide(in: webview)
    let filesPage = SecondaryPage.make(
      like: webview,
      fragment: "/mobile-files",
      handlers: SecondaryPage.Handlers(controller: files.controller(), scheme: scheme)
    )
    let navigation = NavigationController(
      host: host,
      webview: webview,
      layers: NavigationLayers(
        list: list,
        files: pageLayer(filesPage, leading: NavigationController.strip)
      ),
      send: send
    )
    configure(navigation)
    self.navigation = navigation
  }

  private func mountSidebar(
    webview: WKWebView,
    send: @escaping (ChromeIntent) -> Void
  ) -> UIView {
    let page = SecondaryPage.make(
      like: webview,
      fragment: "/mobile-sidebar",
      handlers: SecondaryPage.Handlers(controller: sidebar.controller(), scheme: scheme)
    )
    let list = listLayer(page)
    let menu = RowMenu(page: page, host: list, send: send)
    sidebar.intercept = { type, body, _ in menu.take(type, body) }
    rowMenu = menu
    return list
  }

  private func configure(_ navigation: NavigationController) {
    navigation.chatHeader = corners(.chat)
    navigation.ideHeader = corners(.ide) + [tabStrip].compactMap { $0 }
    navigation.canOpenIde = { [model] in
      model.headers.chat.first { $0.id == "ide" }?.enabled ?? false
    }
    navigation.swipesEnabled = { [model] in model.welcome == nil }
  }

  func applySidebar(_ snapshot: String) {
    sidebar.render(snapshot, to: "__crowbarSidebar")
  }

  func applyAgentsSettings(_ snapshot: String) {
    settings.render(snapshot, to: "__crowbarAgentsSettings")
  }

  func applyFiles(_ snapshot: String) {
    files.render(snapshot, to: "__crowbarFiles")
  }

  func applyHostsSettings(_ snapshot: String) {
    settingsModel.applyHosts(snapshot)
  }

  func applyTabs(_ state: TabsState) {
    tabs.state = state
    tabStrip?.isHidden = state.tabs.isEmpty
  }

  func apply(_ state: ChromeState) {
    _ = settingsPage()
    applyTheme(state)
    model.hostPlace = state.hostPlace ?? "remote"
    model.hostStatus = state.hostStatus ?? "unknown"
    model.welcome = state.welcome
    if let connect = state.connect { model.connect = connect }
    model.headers = state.headers ?? ChromeHeaders(chat: [], ide: [])
    welcome?.layer.zPosition = NavigationController.welcomeZ
    (state.welcome?.visible ?? false ? fadeIn : fadeOut)(welcome)
    let reveal = state.reveal ?? .none
    if reveal != .none { webview?.endEditing(true) }
    navigation?.web(reports: reveal, ack: state.ack ?? 0)
    applySheets(state)
  }

  func settingsPage() -> WKWebView {
    if let settingsWarm { return settingsWarm }
    guard let webview else { return WKWebView() }
    let page = SecondaryPage.make(
      like: webview,
      fragment: "/mobile-settings",
      handlers: SecondaryPage.Handlers(controller: settings.controller(), scheme: scheme)
    )
    settingsWarm = page
    return page
  }
}
