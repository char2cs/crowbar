import SwiftRs
import SwiftUI
import Tauri
import UIKit
import WebKit

class ChromePlugin: Plugin {
  private let model = ChromeModel()
  private var webview: WKWebView?
  private var frontend: UnsafeRawPointer?
  private var overlays: AnyObject?

  override func load(webview: WKWebView) {
    self.webview = webview
    webview.isOpaque = false
    webview.backgroundColor = UIColor(named: "LaunchBackground")
  }

  /// Stores the pointer to the Rust frontend handle that serves app-scheme requests.
  @objc public func attachFrontend(_ invoke: Invoke) throws {
    frontend = try invoke.parseArgs(FrontendHandle.self).pointer
    invoke.resolve()
  }

  /// Applies the chrome state (theme, headers, welcome, connect, settings) to the overlays.
  @objc public func setChrome(_ invoke: Invoke) throws {
    let state = try invoke.parseArgs(ChromeState.self)
    DispatchQueue.main.async {
      if #available(iOS 26.0, *) { self.mounted()?.apply(state) }
      invoke.resolve()
    }
  }

  /// Forwards the sidebar snapshot to the sidebar page.
  @objc public func setSidebar(_ invoke: Invoke) throws {
    let snapshot = invoke.getRawArgs()
    DispatchQueue.main.async {
      if #available(iOS 26.0, *) { self.mounted()?.applySidebar(snapshot) }
      invoke.resolve()
    }
  }

  /// Forwards the agents settings snapshot to the settings page.
  @objc public func setAgentsSettings(_ invoke: Invoke) throws {
    let snapshot = invoke.getRawArgs()
    DispatchQueue.main.async {
      if #available(iOS 26.0, *) { self.mounted()?.applyAgentsSettings(snapshot) }
      invoke.resolve()
    }
  }

  /// Forwards the files snapshot to the files page.
  @objc public func setFiles(_ invoke: Invoke) throws {
    let snapshot = invoke.getRawArgs()
    DispatchQueue.main.async {
      if #available(iOS 26.0, *) { self.mounted()?.applyFiles(snapshot) }
      invoke.resolve()
    }
  }

  /// Forwards the hosts settings snapshot to the settings sheet.
  @objc public func setHostsSettings(_ invoke: Invoke) throws {
    let snapshot = invoke.getRawArgs()
    DispatchQueue.main.async {
      if #available(iOS 26.0, *) { self.mounted()?.applyHostsSettings(snapshot) }
      invoke.resolve()
    }
  }

  /// Opens or closes the model picker sheet and updates its content.
  @objc public func setModelPicker(_ invoke: Invoke) throws {
    let state = try invoke.parseArgs(ModelPickerState.self)
    DispatchQueue.main.async {
      if #available(iOS 26.0, *) { self.mounted()?.applyModelPicker(state) }
      invoke.resolve()
    }
  }

  /// Opens or closes the plus menu sheet and updates its content.
  @objc public func setPlusMenu(_ invoke: Invoke) throws {
    let state = try invoke.parseArgs(PlusMenuState.self)
    DispatchQueue.main.async {
      if #available(iOS 26.0, *) { self.mounted()?.applyPlusMenu(state) }
      invoke.resolve()
    }
  }

  /// Opens or closes the attach sheet and updates its content.
  @objc public func setAttachSheet(_ invoke: Invoke) throws {
    let state = try invoke.parseArgs(AttachSheetState.self)
    DispatchQueue.main.async {
      if #available(iOS 26.0, *) { self.mounted()?.applyAttachSheet(state) }
      invoke.resolve()
    }
  }

  /// Shows or hides the jump-to-latest button.
  @objc public func setJumpToLatest(_ invoke: Invoke) throws {
    let state = try invoke.parseArgs(JumpToLatestState.self)
    DispatchQueue.main.async {
      if #available(iOS 26.0, *) { self.mounted()?.applyJumpToLatest(state) }
      invoke.resolve()
    }
  }

  /// Opens or closes the host picker sheet and updates its content.
  @objc public func setHostPicker(_ invoke: Invoke) throws {
    let state = try invoke.parseArgs(HostPickerState.self)
    DispatchQueue.main.async {
      if #available(iOS 26.0, *) { self.mounted()?.applyHostPicker(state) }
      invoke.resolve()
    }
  }

  /// Updates the tab strip with the given tabs.
  @objc public func setTabs(_ invoke: Invoke) throws {
    let state = try invoke.parseArgs(TabsState.self)
    DispatchQueue.main.async {
      if #available(iOS 26.0, *) { self.mounted()?.applyTabs(state) }
      invoke.resolve()
    }
  }

  /// Presents a native confirmation dialog and resolves with the choice.
  @objc public func confirm(_ invoke: Invoke) throws {
    let options = try invoke.parseArgs(Dialogs.ConfirmOptions.self)
    DispatchQueue.main.async {
      let root = self.webview?.window?.rootViewController
      guard let root else { return invoke.resolve(["confirmed": false]) }
      Dialogs.confirm(options, over: root) { invoke.resolve(["confirmed": $0 == .confirmed]) }
    }
  }

  /// Presents a native text prompt and resolves with the entered value or null.
  @objc public func prompt(_ invoke: Invoke) throws {
    let options = try invoke.parseArgs(Dialogs.PromptOptions.self)
    DispatchQueue.main.async {
      let root = self.webview?.window?.rootViewController
      guard let root else { return invoke.resolve(["value": NSNull()]) }
      Dialogs.prompt(options, over: root) { invoke.resolve(["value": $0 ?? NSNull()]) }
    }
  }

  @available(iOS 26.0, *)
  private func mounted() -> ChromeOverlays? {
    if let overlays = overlays as? ChromeOverlays { return overlays }
    guard let webview, let frontend else { return nil }
    let scheme = AppScheme(frontend: frontend)
    let send: (ChromeIntent) -> Void = { [weak self] intent in
      self?.trigger("intent", data: intent.payload)
    }
    let overlays = ChromeOverlays(model: model, webview: webview, scheme: scheme, send: send)
    self.overlays = overlays
    return overlays
  }
}

@_cdecl("init_plugin_crowbar_chrome")
func initPlugin() -> Plugin {
  return ChromePlugin()
}
