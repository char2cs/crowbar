import SwiftUI
import UIKit
import WebKit

@available(iOS 26.0, *)
final class ChromeOverlays {
  private static let leadingSide = "leading"
  private static let trailingSide = "trailing"
  private let model: ChromeModel
  private let scheme: AppScheme
  private let sidebar: PageChannel
  private let settings: PageChannel
  private let files: PageChannel
  private let picker = ModelPickerModel()
  private let hostPicker = HostPickerModel()
  private let plus = PlusMenuModel()
  private let attach = AttachSheetModel()
  private let settingsModel = SettingsModel()
  private var rowMenu: RowMenu?
  private weak var webview: WKWebView?
  private var navigation: NavigationController?
  private var keyboard: KeyboardFit?
  private lazy var sheets = SheetPresenter(dimming: .undimmed) { [model] in
    let controller = UIHostingController(rootView: ConnectSheet(model: model))
    controller.view.backgroundColor = .clear
    return controller
  } onUserDismiss: { [model] in
    model.send(.dismiss)
  }
  private lazy var settingsSheet = SheetPresenter(dimming: .dimmed, detents: [.large()]) {
    [model, settingsModel, weak self] in
    UIHostingController(
      rootView: SettingsSheet(
        model: settingsModel,
        chrome: model,
        page: self?.settingsPage() ?? WKWebView()
      ) {
        model.send(.of("closeSettings"))
      }
    )
  } onUserDismiss: { [model] in
    model.send(.of("closeSettings"))
  }
  private lazy var pickerSheet = SheetPresenter(dimming: .dimmed, detents: [.large()]) {
    [picker, model] in
    UIHostingController(rootView: ModelPickerSheet(model: picker, chrome: model))
  } onUserDismiss: { [picker] in
    picker.send(.of("closeModelPicker"))
  }
  private lazy var plusSheet: SheetPresenter = SheetPresenter(
    dimming: .dimmed,
    detents: [.medium(), .large()]
  ) { [plus, model, weak self] in
    UIHostingController(
      rootView: PlusMenuSheet(
        model: plus,
        chrome: model,
        pick: { id in
          self?.plusSheet.closeThen { model.send(.of("pickPlusItem", ["id": id])) }
        },
        close: { model.send(.of("closePlusMenu")) }
      )
    )
  } onUserDismiss: { [model] in
    model.send(.of("closePlusMenu"))
  }
  private lazy var attachSheet: SheetPresenter = SheetPresenter(
    dimming: .dimmed,
    detents: [.medium(), .large()]
  ) { [attach, model, weak self] in
    UIHostingController(
      rootView: AttachSheet(
        model: attach,
        chrome: model,
        pick: { id in
          self?.attachSheet.closeThen { model.send(.of("pickAttachSource", ["id": id])) }
        }
      )
    )
  } onUserDismiss: { [model] in
    model.send(.of("closeAttachSheet"))
  }
  private lazy var hostSheet = SheetPresenter(dimming: .dimmed, detents: [.large()]) {
    [hostPicker, model] in
    UIHostingController(rootView: HostPickerSheet(model: hostPicker, chrome: model))
  } onUserDismiss: { [hostPicker] in
    hostPicker.send(.of("closeHostPicker"))
  }
  private lazy var welcome = mount(WelcomeButton(model: model)) { view, host in
    [
      view.centerXAnchor.constraint(equalTo: host.centerXAnchor),
      view.centerYAnchor.constraint(equalTo: host.centerYAnchor, constant: Metric.welcomeLift),
    ]
  }
  private func corner(
    _ header: Header,
    _ side: String
  ) -> UIView? {
    mount(CornerButtons(model: model, header: header, side: side), style: .intrinsicShown) {
      view,
      host in
      [
        view.topAnchor.constraint(equalTo: host.safeAreaLayoutGuide.topAnchor),
        side == Self.leadingSide
          ? view.leadingAnchor.constraint(
            equalTo: host.leadingAnchor,
            constant: GlassCircle.edgeInset
          )
          : view.trailingAnchor.constraint(
            equalTo: host.trailingAnchor,
            constant: -GlassCircle.edgeInset
          ),
      ]
    }
  }

  private let tabs = TabsModel()
  private lazy var tabStrip = mount(TabStrip(model: tabs, chrome: model), style: .fillHidden) {
    view,
    host in
    [
      view.topAnchor.constraint(equalTo: host.safeAreaLayoutGuide.topAnchor),
      view.heightAnchor.constraint(equalToConstant: Metric.tapTarget),
      view.leadingAnchor.constraint(
        equalTo: host.leadingAnchor,
        constant: GlassCircle.edgeInset + GlassCircle.diameter + Spacing.small
      ),
      view.trailingAnchor.constraint(
        equalTo: host.trailingAnchor,
        constant: -GlassCircle.edgeInset - GlassCircle.diameter - Spacing.small
      ),
    ]
  }

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
    settings = PageChannel(
      handler: "settings",
      feeds: [
        "agentsSettingsReady": "__crowbarAgentsSettings",
        "settingsTabReady": "__crowbarSettingsTab",
      ],
      send: send
    )
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
    let page = SecondaryPage.make(
      like: webview,
      fragment: "/mobile-sidebar",
      controller: sidebar.controller(),
      scheme: scheme
    )
    let list = listLayer(page)
    let menu = RowMenu(page: page, host: list, send: send)
    sidebar.intercept = { type, body, _ in menu.take(type, body) }
    rowMenu = menu
    keyboard = KeyboardFit(webview: webview)
    keyboard?.fitted = { [weak self] in self?.placeJumpButton() }
    paintSurface(under: webview, host: host)
    FormAccessory.hide(in: webview)
    let filesPage = SecondaryPage.make(
      like: webview,
      fragment: "/mobile-files",
      controller: files.controller(),
      scheme: scheme
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
    navigation.chatHeader = [corner(.chat, Self.leadingSide), corner(.chat, Self.trailingSide)].compactMap { $0 }
    navigation.ideHeader =
      [corner(.ide, Self.leadingSide), corner(.ide, Self.trailingSide), tabStrip].compactMap { $0 }
    navigation.canOpenIde = { [model] in
      model.headers.chat.first { $0.id == "ide" }?.enabled ?? false
    }
    navigation.swipesEnabled = { [model] in model.welcome == nil }
    self.navigation = navigation
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

  func applyModelPicker(_ state: ModelPickerState) {
    picker.apply(state)
    pickerSheet.update(state.open ? .open : .closed, over: webview?.window?.rootViewController)
  }

  func applyPlusMenu(_ state: PlusMenuState) {
    if state.open { plus.state = state }
    plusSheet.update(state.open ? .open : .closed, over: webview?.window?.rootViewController)
  }

  func applyAttachSheet(_ state: AttachSheetState) {
    if state.open { attach.state = state }
    attachSheet.update(state.open ? .open : .closed, over: webview?.window?.rootViewController)
  }

  func applyJumpToLatest(_ state: JumpToLatestState) {
    guard let view = jumpButton else { return }
    if state.visible { model.jumpOwner = state.owner }
    if !state.visible && state.owner != model.jumpOwner { return }
    guard state.visible else { return hideJumpButton(view) }
    model.jumpLabel = state.label
    jumpBottom = CGFloat(state.bottom)
    showJumpButton(view)
  }

  private func showJumpButton(_ view: UIView) {
    if view.isHidden || view.alpha == 0 { prepareJumpButton(view) }
    UIView.animate(
      withDuration: Metric.fastDuration,
      delay: 0,
      options: [.curveEaseOut, .beginFromCurrentState]
    ) {
      self.placeJumpButton()
      view.alpha = 1
      view.transform = .identity
    }
  }

  private func prepareJumpButton(_ view: UIView) {
    placeJumpButton()
    view.transform = CGAffineTransform(scaleX: Metric.popScale, y: Metric.popScale)
    view.isHidden = false
  }

  private func hideJumpButton(_ view: UIView) {
    UIView.animate(
      withDuration: Metric.fastDuration,
      delay: 0,
      options: [.curveEaseIn, .beginFromCurrentState],
      animations: {
        view.alpha = 0
        view.transform = CGAffineTransform(scaleX: Metric.popScale, y: Metric.popScale)
      },
      completion: { _ in view.isHidden = view.alpha == 0 }
    )
  }

  private var jumpBottom: CGFloat = 0

  private func placeJumpButton() {
    guard let view = jumpButton, let webview else { return }
    view.center = CGPoint(
      x: webview.bounds.midX,
      y: webview.bounds.height - jumpBottom - GlassCircle.diameter / 2
    )
  }

  private lazy var jumpButton: UIView? = {
    guard let webview else { return nil }
    let controller = UIHostingController(
      rootView: JumpToLatestButton(model: model)
    )
    controller.view.backgroundColor = .clear
    let size = GlassCircle.diameter
    controller.view.frame = CGRect(x: 0, y: 0, width: size, height: size)
    controller.view.alpha = 0
    controller.view.isHidden = true
    webview.addSubview(controller.view)
    hosted.append(controller)
    return controller.view
  }()

  func applyHostPicker(_ state: HostPickerState) {
    hostPicker.state = state
    hostSheet.update(state.open ? .open : .closed, over: webview?.window?.rootViewController)
  }

  private func listLayer(_ page: WKWebView) -> UIView {
    let layer = pageLayer(page, trailing: NavigationController.strip)
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

  private func paintSurface(
    under webview: WKWebView,
    host: UIView
  ) {
    surfaces.append(
      webview.observe(\.themeColor, options: [.initial, .new]) { [weak host] page, _ in
        let surface = page.themeColor ?? UIColor(named: "LaunchBackground") ?? .systemBackground
        host?.backgroundColor = surface
        host?.window?.backgroundColor = surface
        page.backgroundColor = surface
        page.scrollView.backgroundColor = surface
        page.underPageBackgroundColor = surface
      }
    )
  }

  private func pageLayer(
    _ page: WKWebView,
    leading: CGFloat = 0,
    trailing: CGFloat = 0
  ) -> UIView {
    let layer = UIView()
    surfaces.append(
      page.observe(\.themeColor, options: [.initial, .new]) { [weak layer] page, _ in
        let ground = page.themeColor ?? .systemBackground
        layer?.backgroundColor = ground
        page.underPageBackgroundColor = ground
        page.scrollView.backgroundColor = ground
      }
    )
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
    _ = settingsPage()
    // PLATFORM: ios: The app's theme, not the device's: sheets, glass and keyboard match the web beneath.
    webview?.window?.overrideUserInterfaceStyle =
      switch state.appearance {
      case "light": .light
      case "dark": .dark
      default: .unspecified
      }
    webview?.window?.backgroundColor = webview?.superview?.backgroundColor
    model.accent = state.accent.flatMap { Color(css: $0) }
    if let ground = state.brandGround.flatMap({ Color(css: $0) }) { model.brandGround = ground }
    if let palette = state.palette.flatMap(ChromePalette.init) { model.palette = palette }
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
    settingsModel.themeMode = state.appearance ?? "system"
    settingsModel.state = state.settings
    settingsSheet.update(
      state.settings?.open ?? false ? .open : .closed,
      over: webview?.window?.rootViewController
    )
    sheets.update(
      state.connect?.open ?? false ? .open : .closed,
      dismissal: state.connect?.busy ?? false ? .blocked : .allowed,
      over: webview?.window?.rootViewController
    )
  }

  private func settingsPage() -> WKWebView {
    if let settingsWarm { return settingsWarm }
    guard let webview else { return WKWebView() }
    let page = SecondaryPage.make(
      like: webview,
      fragment: "/mobile-settings",
      controller: settings.controller(),
      scheme: scheme
    )
    settingsWarm = page
    return page
  }

  private var settingsWarm: WKWebView?

  private func fadeIn(_ view: UIView?) {
    guard let view else { return }
    view.isHidden = false
    UIView.animate(
      withDuration: Metric.fastDuration,
      animations: { view.alpha = 1 },
      completion: { _ in view.isHidden = view.alpha == 0 }
    )
  }

  private func fadeOut(_ view: UIView?) {
    guard let view else { return }
    UIView.animate(
      withDuration: Metric.fastDuration,
      animations: { view.alpha = 0 },
      completion: { _ in view.isHidden = view.alpha == 0 }
    )
  }

  private func mount<Content: View>(
    _ content: Content,
    style: MountStyle = .intrinsicHidden,
    place: (UIView, UIView) -> [NSLayoutConstraint]
  ) -> UIView? {
    guard let host = webview?.superview ?? webview else { return nil }
    let controller = UIHostingController(rootView: content)
    controller.view.backgroundColor = .clear
    if style.sized { controller.sizingOptions = .intrinsicContentSize }
    controller.view.translatesAutoresizingMaskIntoConstraints = false
    controller.view.alpha = 0
    controller.view.isHidden = style.hidden
    host.addSubview(controller.view)
    NSLayoutConstraint.activate(place(controller.view, host))
    hosted.append(controller)
    return controller.view
  }

  private var hosted: [UIViewController] = []
  private var surfaces: [NSKeyValueObservation] = []
}
