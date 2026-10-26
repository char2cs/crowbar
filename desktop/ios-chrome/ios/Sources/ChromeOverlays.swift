import SwiftUI
import UIKit
import WebKit

/// The native views drawn over the webview, each sized to its own content and hidden while it
/// has nothing to show, so every other touch reaches the web.
@available(iOS 26.0, *)
final class ChromeOverlays {
  private let model: ChromeModel
  private let sidebar: PageChannel
  private let settings: PageChannel
  private let files: PageChannel
  private var filesDrawer: FilesDrawer?
  private let picker = ModelPickerModel()
  private var rowMenu: RowMenu?
  private weak var webview: WKWebView?
  private var reveal: RevealController?
  private var keyboard: KeyboardFit?
  private lazy var sheets = SheetPresenter(undimmed: true) { [model] in
    let controller = UIHostingController(rootView: ConnectSheet(model: model))
    controller.view.backgroundColor = .clear
    return controller
  } onUserDismiss: { [model] in
    model.send(.dismiss)
  }
  private lazy var settingsSheet = SheetPresenter(undimmed: false, detents: [.large()]) {
    [model, weak self] in
    UIHostingController(
      rootView: SettingsSheet(page: self?.settingsPage() ?? WKWebView()) {
        model.send(.of("closeSettings"))
      })
  } onUserDismiss: { [model] in
    model.send(.of("closeSettings"))
  }
  private lazy var pickerSheet = SheetPresenter(undimmed: false) { [picker, model] in
    UIHostingController(rootView: ModelPickerSheet(model: picker, chrome: model))
  } onUserDismiss: { [picker] in
    picker.send(.of("closeModelPicker"))
  }
  private lazy var welcome = mount(WelcomeButton(model: model)) { view, host in
    // Below the web's centred brand, which sits 3rem above the middle.
    [
      view.centerXAnchor.constraint(equalTo: host.centerXAnchor),
      view.centerYAnchor.constraint(equalTo: host.centerYAnchor, constant: 84),
    ]
  }
  private lazy var leading = mount(CornerButtons(model: model, side: "leading")) { view, host in
    [
      view.topAnchor.constraint(equalTo: host.safeAreaLayoutGuide.topAnchor),
      view.leadingAnchor.constraint(equalTo: host.leadingAnchor, constant: GlassCircle.edgeInset),
    ]
  }
  private lazy var trailing = mount(CornerButtons(model: model, side: "trailing")) { view, host in
    [
      view.topAnchor.constraint(equalTo: host.safeAreaLayoutGuide.topAnchor),
      view.trailingAnchor.constraint(equalTo: host.trailingAnchor, constant: -GlassCircle.edgeInset),
    ]
  }

  private let tabs = TabsModel()
  private var shown: Reveal = .none
  private lazy var tabStrip = mount(TabStrip(model: tabs), sized: false) { view, host in
    // Between the IDE's Back and Files circles, on their row.
    [
      view.topAnchor.constraint(equalTo: host.safeAreaLayoutGuide.topAnchor),
      view.heightAnchor.constraint(equalToConstant: 44),
      view.leadingAnchor.constraint(
        equalTo: host.leadingAnchor, constant: GlassCircle.edgeInset + 44 + 8),
      view.trailingAnchor.constraint(
        equalTo: host.trailingAnchor, constant: -GlassCircle.edgeInset - 44 - 8),
    ]
  }

  init(model: ChromeModel, webview: WKWebView, send: @escaping (ChromeIntent) -> Void) {
    self.model = model
    self.webview = webview
    sidebar = PageChannel(handler: "sidebar", feeds: ["sidebarReady": "__crowbarSidebar"], send: send)
    settings = PageChannel(
      handler: "settings",
      feeds: [
        "agentsSettingsReady": "__crowbarAgentsSettings",
        "hostsSettingsReady": "__crowbarHostsSettings",
      ], send: send)
    files = PageChannel(handler: "files", feeds: ["filesReady": "__crowbarFiles"], send: send)
    picker.send = send
    tabs.send = send
    model.present = { [weak webview] dialog in
      guard let root = webview?.window?.rootViewController else { return }
      Dialogs.topmost(root).present(dialog, animated: true)
    }
    if let host = webview.superview {
      let page = SecondaryPage.make(
        like: webview, fragment: "/mobile-sidebar", controller: sidebar.controller())
      let list = listLayer(page)
      let menu = RowMenu(page: page, on: list, send: send)
      sidebar.intercept = { type, body, _ in menu.take(type, body) }
      rowMenu = menu
      keyboard = KeyboardFit(webview: webview)
      paintSurface(under: webview, host: host)
      FormAccessory.hide(in: webview)
      reveal = RevealController(host: host, webview: webview, list: list, send: send)
      let filesPage = SecondaryPage.make(
        like: webview, fragment: "/mobile-files", controller: files.controller())
      filesDrawer = FilesDrawer(
        host: host, webview: webview, layer: pageLayer(filesPage, leading: RevealController.strip),
        send: send)
    }
    reveal?.riders = [leading, trailing].compactMap { $0 }
    filesDrawer?.riders = [leading, trailing, tabStrip].compactMap { $0 }
    model.send = { [weak self] intent in
      // The chat is frozen before the web swaps it for the IDE, so it can slide away.
      if intent.payload["id"] as? String == "ide" { self?.reveal?.freezeChat() }
      send(intent)
    }
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
    settings.render(snapshot, to: "__crowbarHostsSettings")
  }

  func applyContext(_ state: ContextState) {
    model.context = state
  }

  func applyTabs(_ state: TabsState) {
    tabs.state = state
    showTabs()
  }

  private func showTabs() {
    show(tabStrip, shown == .ide && !tabs.state.tabs.isEmpty)
  }

  func applyModelPicker(_ state: ModelPickerState) {
    picker.apply(state)
    pickerSheet.update(open: state.open, busy: false, over: webview?.window?.rootViewController)
  }

  /// The chat list: its page clear of the card's strip on the right, under its native foot.
  private func listLayer(_ page: WKWebView) -> UIView {
    let layer = pageLayer(page, trailing: RevealController.strip)
    let foot = UIHostingController(rootView: ChatListFoot(model: model))
    foot.view.backgroundColor = .clear
    foot.sizingOptions = .intrinsicContentSize
    foot.view.translatesAutoresizingMaskIntoConstraints = false
    layer.addSubview(foot.view)
    NSLayoutConstraint.activate([
      foot.view.leadingAnchor.constraint(equalTo: page.leadingAnchor),
      foot.view.trailingAnchor.constraint(equalTo: page.trailingAnchor),
      foot.view.bottomAnchor.constraint(equalTo: layer.safeAreaLayoutGuide.bottomAnchor),
    ])
    hosted.append(foot)
    return layer
  }

  /// Whatever the webview uncovers (KeyboardFit shrinking it, the keyboard's rounded top, a
  /// scroll overshoot) shows the page's own surface rather than black.
  private func paintSurface(under webview: WKWebView, host: UIView) {
    surfaces.append(
      webview.observe(\.themeColor, options: [.initial, .new]) { [weak host] page, _ in
        let surface = page.themeColor ?? UIColor(named: "LaunchBackground") ?? .systemBackground
        host?.backgroundColor = surface
        host?.window?.backgroundColor = surface
        page.backgroundColor = surface
        page.scrollView.backgroundColor = surface
        page.underPageBackgroundColor = surface
      })
  }

  /// A drawer's layer: its page, inset from the side the card's strip covers, on the page's own
  /// surface (its theme-color), which also shows behind the card's strip and corners.
  private func pageLayer(
    _ page: WKWebView, leading: CGFloat = 0, trailing: CGFloat = 0
  ) -> UIView {
    let layer = UIView()
    surfaces.append(
      page.observe(\.themeColor, options: [.initial, .new]) { [weak layer] page, _ in
        layer?.backgroundColor = page.themeColor ?? .systemBackground
      })
    page.translatesAutoresizingMaskIntoConstraints = false
    layer.addSubview(page)
    NSLayoutConstraint.activate([
      page.topAnchor.constraint(equalTo: layer.topAnchor),
      page.bottomAnchor.constraint(equalTo: layer.bottomAnchor),
      page.leadingAnchor.constraint(equalTo: layer.leadingAnchor, constant: leading),
      page.trailingAnchor.constraint(equalTo: layer.trailingAnchor, constant: -trailing),
    ])
    return layer
  }

  func apply(_ state: ChromeState) {
    // The web is up and talking: the settings page can load behind it now.
    _ = settingsPage()
    // The app's theme, not the device's: sheets, glass and keyboard match the web beneath.
    webview?.window?.overrideUserInterfaceStyle =
      switch state.appearance {
      case "light": .light
      case "dark": .dark
      default: .unspecified
      }
    // The window only exists once mounted; the surface observer may have fired before that.
    webview?.window?.backgroundColor = webview?.superview?.backgroundColor
    model.accent = state.accent.flatMap { Color(css: $0) }
    // Re-sent on every theme change, so it is always the current theme's green.
    if let ground = state.brandGround.flatMap({ Color(css: $0) }) { model.brandGround = ground }
    if let palette = state.palette.flatMap(ChromePalette.init) { model.palette = palette }
    model.welcome = state.welcome
    if let connect = state.connect { model.connect = connect }
    let buttons = state.buttons ?? []
    model.buttons = buttons
    show(welcome, state.welcome?.visible ?? false)
    show(leading, buttons.contains { $0.side == "leading" })
    show(trailing, buttons.contains { $0.side == "trailing" })
    // Files are a drawer over the IDE: under it, the IDE stays revealed.
    shown = state.reveal ?? .none
    // A drawer or the IDE over the chat leaves no keyboard up for the composer beneath.
    if shown != .none { webview?.endEditing(true) }
    showTabs()
    reveal?.locked = shown == .files
    reveal?.apply(shown == .files ? .ide : shown)
    filesDrawer?.enabled = shown == .ide || shown == .files
    filesDrawer?.apply(open: shown == .files)
    settingsSheet.update(
      open: state.settings?.open ?? false, busy: false, over: webview?.window?.rootViewController)
    sheets.update(
      open: state.connect?.open ?? false,
      busy: state.connect?.busy ?? false,
      over: webview?.window?.rootViewController)
  }

  /// The web's own settings, rendered without IPC from shared storage and pushed snapshots.
  /// Made once, as soon as the web is up, so the sheet opens on a page that has already loaded.
  private func settingsPage() -> WKWebView {
    if let settingsWarm { return settingsWarm }
    guard let webview else { return WKWebView() }
    let page = SecondaryPage.make(
      like: webview, fragment: "/mobile-settings", controller: settings.controller())
    settingsWarm = page
    return page
  }

  private var settingsWarm: WKWebView?

  private func show(_ view: UIView?, _ visible: Bool) {
    guard let view else { return }
    if visible { view.isHidden = false }
    UIView.animate(
      withDuration: 0.2,
      animations: { view.alpha = visible ? 1 : 0 },
      completion: { _ in view.isHidden = view.alpha == 0 })
  }

  private func mount<Content: View>(
    _ content: Content, sized: Bool = true, place: (UIView, UIView) -> [NSLayoutConstraint]
  ) -> UIView? {
    guard let host = webview?.superview ?? webview else { return nil }
    let controller = UIHostingController(rootView: content)
    controller.view.backgroundColor = .clear
    if sized { controller.sizingOptions = .intrinsicContentSize }
    controller.view.translatesAutoresizingMaskIntoConstraints = false
    controller.view.alpha = 0
    controller.view.isHidden = true
    host.addSubview(controller.view)
    NSLayoutConstraint.activate(place(controller.view, host))
    hosted.append(controller)
    return controller.view
  }

  private var hosted: [UIViewController] = []
  private var surfaces: [NSKeyValueObservation] = []
}
