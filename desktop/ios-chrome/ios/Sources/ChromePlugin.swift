import SwiftRs
import SwiftUI
import Tauri
import UIKit
import WebKit

class ChromePlugin: Plugin {
  private let model = ChromeModel()
  private var webview: WKWebView?
  private var overlays: AnyObject?

  override func load(webview: WKWebView) {
    self.webview = webview
    // The launch screen's colour until the page paints its own: no white flash between them.
    webview.isOpaque = false
    webview.backgroundColor = UIColor(named: "LaunchBackground")
  }

  @objc public func setChrome(_ invoke: Invoke) throws {
    let state = try invoke.parseArgs(ChromeState.self)
    DispatchQueue.main.async {
      if #available(iOS 26.0, *) { self.mounted()?.apply(state) }
      invoke.resolve()
    }
  }

  @objc public func setSidebar(_ invoke: Invoke) throws {
    let snapshot = invoke.getRawArgs()
    DispatchQueue.main.async {
      if #available(iOS 26.0, *) { self.mounted()?.applySidebar(snapshot) }
      invoke.resolve()
    }
  }

  @objc public func setAgentsSettings(_ invoke: Invoke) throws {
    let snapshot = invoke.getRawArgs()
    DispatchQueue.main.async {
      if #available(iOS 26.0, *) { self.mounted()?.applyAgentsSettings(snapshot) }
      invoke.resolve()
    }
  }

  @objc public func setFiles(_ invoke: Invoke) throws {
    let snapshot = invoke.getRawArgs()
    DispatchQueue.main.async {
      if #available(iOS 26.0, *) { self.mounted()?.applyFiles(snapshot) }
      invoke.resolve()
    }
  }

  @objc public func setHostsSettings(_ invoke: Invoke) throws {
    let snapshot = invoke.getRawArgs()
    DispatchQueue.main.async {
      if #available(iOS 26.0, *) { self.mounted()?.applyHostsSettings(snapshot) }
      invoke.resolve()
    }
  }

  @objc public func setModelPicker(_ invoke: Invoke) throws {
    let state = try invoke.parseArgs(ModelPickerState.self)
    DispatchQueue.main.async {
      if #available(iOS 26.0, *) { self.mounted()?.applyModelPicker(state) }
      invoke.resolve()
    }
  }

  @objc public func setContext(_ invoke: Invoke) throws {
    let state = try invoke.parseArgs(ContextState.self)
    DispatchQueue.main.async {
      if #available(iOS 26.0, *) { self.mounted()?.applyContext(state) }
      invoke.resolve()
    }
  }

  @objc public func setTabs(_ invoke: Invoke) throws {
    let state = try invoke.parseArgs(TabsState.self)
    DispatchQueue.main.async {
      if #available(iOS 26.0, *) { self.mounted()?.applyTabs(state) }
      invoke.resolve()
    }
  }

  @objc public func confirm(_ invoke: Invoke) throws {
    let options = try invoke.parseArgs(Dialogs.ConfirmOptions.self)
    DispatchQueue.main.async {
      guard let root = self.webview?.window?.rootViewController else {
        return invoke.resolve(["confirmed": false])
      }
      Dialogs.confirm(options, over: root) { invoke.resolve(["confirmed": $0]) }
    }
  }

  @objc public func prompt(_ invoke: Invoke) throws {
    let options = try invoke.parseArgs(Dialogs.PromptOptions.self)
    DispatchQueue.main.async {
      guard let root = self.webview?.window?.rootViewController else {
        return invoke.resolve(["value": NSNull()])
      }
      Dialogs.prompt(options, over: root) { invoke.resolve(["value": $0 ?? NSNull()]) }
    }
  }

  @available(iOS 26.0, *)
  private func mounted() -> ChromeOverlays? {
    if let overlays = overlays as? ChromeOverlays { return overlays }
    guard let webview else { return nil }
    let overlays = ChromeOverlays(model: model, webview: webview) { [weak self] intent in
      self?.trigger("intent", data: intent.payload)
    }
    self.overlays = overlays
    return overlays
  }
}

@_cdecl("init_plugin_crowbar_chrome")
func initPlugin() -> Plugin {
  return ChromePlugin()
}
