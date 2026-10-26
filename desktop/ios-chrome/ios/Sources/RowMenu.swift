import UIKit
import WebKit

/// A chat-list row's long-press menu, native. The page owns the hold and asks for the menu
/// (`rowMenu`) once the finger has rested on a row; an invisible button over that row presents
/// it. A pick goes to the main webview as `rowAction`; destructive ones may ask first.
@available(iOS 26.0, *)
final class RowMenu: NSObject {
  struct Action {
    let id: String
    let title: String
    let symbol: String?
    let destructive: Bool
    let confirm: (title: String, message: String)?
  }

  /// A button that reports when its menu is on screen and when it has finished leaving.
  private final class Anchor: UIButton {
    var onDisplay: (() -> Void)?
    var onGone: (() -> Void)?

    override func contextMenuInteraction(
      _ interaction: UIContextMenuInteraction, willDisplayMenuFor configuration: UIContextMenuConfiguration,
      animator: (any UIContextMenuInteractionAnimating)?
    ) {
      super.contextMenuInteraction(
        interaction, willDisplayMenuFor: configuration, animator: animator)
      onDisplay?()
    }

    override func contextMenuInteraction(
      _ interaction: UIContextMenuInteraction, willEndFor configuration: UIContextMenuConfiguration,
      animator: (any UIContextMenuInteractionAnimating)?
    ) {
      super.contextMenuInteraction(interaction, willEndFor: configuration, animator: animator)
      guard let animator else { return onGone?() ?? () }
      animator.addCompletion { [weak self] in self?.onGone?() }
    }
  }

  private weak var page: WKWebView?
  private weak var host: UIView?
  private let send: (ChromeIntent) -> Void
  /// The menu is on screen or still animating away; nothing else can be presented over it.
  private var menuUp = false
  /// A confirmation picked from the menu, shown once the menu has finished leaving.
  private var pendingConfirm: UIAlertController?

  init(page: WKWebView, host: UIView, send: @escaping (ChromeIntent) -> Void) {
    self.page = page
    self.host = host
    self.send = send
  }

  /// A page message this menu takes; anything else is not its business.
  func take(_ type: String, _ body: [String: Any]) -> Bool {
    switch type {
    case "rowDrag":
      if body["active"] as? Bool ?? false { Haptics.lift() } else { Haptics.tap() }
      return true
    case "rowMenu":
      guard let rowId = body["rowId"] as? String else { return true }
      let rect = body["rect"] as? [String: Double] ?? [:]
      open(
        row: rowId,
        rect: CGRect(
          x: rect["x"] ?? 0, y: rect["y"] ?? 0, width: rect["width"] ?? 0,
          height: rect["height"] ?? 0),
        actions: (body["actions"] as? [[String: Any]] ?? []).compactMap(Self.action))
      return true
    default:
      return false
    }
  }

  static func action(_ raw: [String: Any]) -> Action? {
    guard let id = raw["id"] as? String, let title = raw["title"] as? String else { return nil }
    let confirm = (raw["confirm"] as? [String: Any]).map {
      (title: $0["title"] as? String ?? title, message: $0["message"] as? String ?? "")
    }
    return Action(
      id: id, title: title, symbol: raw["symbol"] as? String,
      destructive: raw["destructive"] as? Bool ?? false, confirm: confirm)
  }

  private func open(row: String, rect: CGRect, actions: [Action]) {
    guard !menuUp, !actions.isEmpty, let page, let host else { return }
    let anchor = Anchor(frame: page.convert(rect, to: host))
    anchor.menu = menu(actions, row: row)
    anchor.showsMenuAsPrimaryAction = true
    anchor.onDisplay = { [weak self] in self?.menuUp = true }
    anchor.onGone = { [weak self, weak anchor] in
      anchor?.removeFromSuperview()
      self?.menuGone()
    }
    host.addSubview(anchor)
    Haptics.lift()
    anchor.sendActions(for: .menuActionTriggered)
  }

  private func menu(_ actions: [Action], row: String) -> UIMenu {
    let item = { [weak self] (action: Action) in
      UIAction(
        title: action.title, image: action.symbol.flatMap { UIImage(systemName: $0) },
        attributes: action.destructive ? .destructive : []
      ) { [weak self] _ in self?.pick(action, row: row) }
    }
    // Destructive actions sit apart, last, as the system's own menus keep them.
    let safe = actions.filter { !$0.destructive }.map(item)
    let destructive = actions.filter(\.destructive).map(item)
    return UIMenu(children: safe + (destructive.isEmpty ? [] : [
      UIMenu(options: .displayInline, children: destructive)
    ]))
  }

  private func menuGone() {
    menuUp = false
    if let alert = pendingConfirm {
      pendingConfirm = nil
      present(alert)
    }
  }

  private func pick(_ action: Action, row: String) {
    let picked = ChromeIntent.of("rowAction", ["rowId": row, "actionId": action.id])
    guard let confirm = action.confirm else { return send(picked) }
    let alert = UIAlertController(
      title: confirm.title, message: confirm.message, preferredStyle: .alert)
    alert.addAction(UIAlertAction(title: "Cancel", style: .cancel))
    alert.addAction(
      UIAlertAction(title: action.title, style: action.destructive ? .destructive : .default) {
        [weak self] _ in self?.send(picked)
      })
    // Presenting while the menu is still dismissing is silently dropped by UIKit.
    if menuUp { pendingConfirm = alert } else { present(alert) }
  }

  private func present(_ alert: UIAlertController) {
    guard let presenter = page?.window?.rootViewController else { return }
    Dialogs.topmost(presenter).present(alert, animated: true)
  }
}
