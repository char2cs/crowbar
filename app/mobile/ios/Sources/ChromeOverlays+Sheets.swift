import SwiftUI
import UIKit
import WebKit

@available(iOS 26.0, *)
extension ChromeOverlays {
  func makeConnectSheet() -> SheetPresenter {
    SheetPresenter(dimming: .undimmed) { [model] in
      let controller = UIHostingController(rootView: ConnectSheet(model: model))
      controller.view.backgroundColor = .clear
      return controller
    } onUserDismiss: { [model] in
      model.send(.dismiss)
    }
  }

  func makeSettingsSheet() -> SheetPresenter {
    SheetPresenter(dimming: .dimmed, detents: [.large()]) { [model, settingsModel, weak self] in
      UIHostingController(
        rootView: SettingsSheet(
          model: settingsModel,
          chrome: model,
          page: self?.settingsPage() ?? WKWebView()
        ) { model.send(.of("closeSettings")) }
      )
    } onUserDismiss: { [model] in
      model.send(.of("closeSettings"))
    }
  }

  func makePickerSheet() -> SheetPresenter {
    SheetPresenter(dimming: .dimmed, detents: [.large()]) { [picker, model] in
      UIHostingController(rootView: ModelPickerSheet(model: picker, chrome: model))
    } onUserDismiss: { [picker] in
      picker.send(.of("closeModelPicker"))
    }
  }

  func makePlusSheet() -> SheetPresenter {
    SheetPresenter(
      dimming: .dimmed,
      detents: [.medium(), .large()]
    ) { [plus, model, weak self] in
      UIHostingController(
        rootView: PlusMenuSheet(
          model: plus,
          chrome: model,
          pick: { id in self?.pickPlusItem(id) },
          close: { model.send(.of("closePlusMenu")) }
        )
      )
    } onUserDismiss: { [model] in
      model.send(.of("closePlusMenu"))
    }
  }

  func makeAttachSheet() -> SheetPresenter {
    SheetPresenter(
      dimming: .dimmed,
      detents: [.medium(), .large()]
    ) { [attach, model, weak self] in
      UIHostingController(
        rootView: AttachSheet(
          model: attach,
          chrome: model,
          pick: { id in self?.pickAttachSource(id) }
        )
      )
    } onUserDismiss: { [model] in
      model.send(.of("closeAttachSheet"))
    }
  }

  func pickPlusItem(_ id: String) {
    plusSheet.closeThen { [model] in model.send(.of("pickPlusItem", ["id": id])) }
  }

  func pickAttachSource(_ id: String) {
    attachSheet.closeThen { [model] in model.send(.of("pickAttachSource", ["id": id])) }
  }

  func makeHostSheet() -> SheetPresenter {
    SheetPresenter(dimming: .dimmed, detents: [.large()]) { [hostPicker, model] in
      UIHostingController(rootView: HostPickerSheet(model: hostPicker, chrome: model))
    } onUserDismiss: { [hostPicker] in
      hostPicker.send(.of("closeHostPicker"))
    }
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

  func applyHostPicker(_ state: HostPickerState) {
    hostPicker.state = state
    hostSheet.update(state.open ? .open : .closed, over: webview?.window?.rootViewController)
  }

  func applyTheme(_ state: ChromeState) {
    // PLATFORM: ios: app theme, not the device's, so sheets and glass match the web.
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
  }

  func applySheets(_ state: ChromeState) {
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
}
