import UIKit
import WebKit

/// A chat-list row's menu, native: the page's ⋯ button on a row sends `rowMenu` with the row,
/// the button's rect and the row's actions, and a sheet answers at that button. A pick goes to
/// the main webview as `rowAction`; destructive ones may ask first. A hold on a row is the
/// page's drag alone (`rowDrag`), which closes a sheet that is up.
@available(iOS 26.0, *)
final class RowMenu {
  struct Action {
    let id: String
    let title: String
    let destructive: Bool
    let confirm: (title: String, message: String)?
  }

  private weak var page: WKWebView?
  private let send: (ChromeIntent) -> Void
  private weak var sheet: UIAlertController?
  private var dragging = false

  init(page: WKWebView, send: @escaping (ChromeIntent) -> Void) {
    self.page = page
    self.send = send
  }

  private func drag(_ active: Bool) {
    guard active != dragging else { return }
    dragging = active
    if active {
      sheet?.dismiss(animated: false)
      Haptics.lift()
    } else {
      Haptics.tap()
    }
  }

  /// A page message this menu takes; anything else is not its business.
  func take(_ type: String, _ body: [String: Any]) -> Bool {
    if type == "rowDrag" {
      drag(body["active"] as? Bool ?? false)
      return true
    }
    guard type == "rowMenu" else { return false }
    guard let rowId = body["rowId"] as? String else { return true }
    let rect = body["rect"] as? [String: Double] ?? [:]
    show(
      row: rowId,
      at: CGRect(
        x: rect["x"] ?? 0, y: rect["y"] ?? 0, width: rect["width"] ?? 0,
        height: rect["height"] ?? 0),
      actions: (body["actions"] as? [[String: Any]] ?? []).compactMap(Self.action))
    return true
  }

  static func action(_ raw: [String: Any]) -> Action? {
    guard let id = raw["id"] as? String, let title = raw["title"] as? String else { return nil }
    let confirm = (raw["confirm"] as? [String: Any]).map {
      (title: $0["title"] as? String ?? title, message: $0["message"] as? String ?? "")
    }
    return Action(
      id: id, title: title, destructive: raw["destructive"] as? Bool ?? false, confirm: confirm)
  }

  private func show(row: String, at rect: CGRect, actions: [Action]) {
    guard let page, !actions.isEmpty, let presenter = page.window?.rootViewController else {
      return
    }
    let menu = UIAlertController(title: nil, message: nil, preferredStyle: .actionSheet)
    // Destructive actions sit last, as the system's own sheets keep them.
    for action in actions.filter({ !$0.destructive }) + actions.filter(\.destructive) {
      let item = UIAlertAction(
        title: action.title, style: action.destructive ? .destructive : .default
      ) { [weak self] _ in self?.pick(action, row: row) }
      menu.addAction(item)
    }
    menu.addAction(UIAlertAction(title: "Cancel", style: .cancel))
    menu.popoverPresentationController?.sourceView = page
    menu.popoverPresentationController?.sourceRect = rect
    sheet = menu
    Dialogs.topmost(presenter).present(menu, animated: true)
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
    guard let presenter = page?.window?.rootViewController else { return }
    Dialogs.topmost(presenter).present(alert, animated: true)
  }
}
