import UIKit
import WebKit

/// A chat-list row's long-press menu, native: the page reports the row it is touched on as the
/// touch starts (`rowPress`), and the system's own long press lifts that row and shows its
/// actions. A pick goes to the main webview as `rowAction`; destructive ones may ask first.
/// While the page carries a row to reorder it (`rowDrag`), the menu stands aside.
@available(iOS 26.0, *)
final class RowMenu: NSObject, UIContextMenuInteractionDelegate {
  struct Action {
    let id: String
    let title: String
    let symbol: String?
    let destructive: Bool
    let confirm: (title: String, message: String)?
  }

  private struct Press {
    let rowId: String
    let rect: CGRect
    let actions: [Action]
  }

  private weak var page: WKWebView?
  private let send: (ChromeIntent) -> Void
  private var press: Press?
  private var interaction: UIContextMenuInteraction?
  /// The page is carrying a row (`rowDrag`): a hold that moved is a reorder, not a menu.
  private var dragging = false
  /// The menu is on screen or still animating away; nothing else can be presented over it.
  private var menuUp = false
  /// A confirmation picked from the menu, shown once the menu has finished leaving.
  private var pendingConfirm: UIAlertController?

  init(page: WKWebView, on layer: UIView, send: @escaping (ChromeIntent) -> Void) {
    self.page = page
    self.send = send
    super.init()
    let interaction = UIContextMenuInteraction(delegate: self)
    layer.addInteraction(interaction)
    self.interaction = interaction
  }

  private func restart() {
    guard let interaction, let layer = interaction.view else { return }
    layer.removeInteraction(interaction)
    layer.addInteraction(interaction)
  }

  private func drag(_ active: Bool) {
    guard active != dragging else { return }
    dragging = active
    if active {
      // A menu on screen closes; one still pending (the system's hold not yet fired) is
      // cancelled by re-adding the interaction, which resets its recognizer mid-touch.
      interaction?.dismissMenu()
      restart()
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
    guard type == "rowPress" else { return false }
    let rect = body["rect"] as? [String: Double] ?? [:]
    press = (body["rowId"] as? String).map { rowId in
      Press(
        rowId: rowId,
        rect: CGRect(
          x: rect["x"] ?? 0, y: rect["y"] ?? 0, width: rect["width"] ?? 0,
          height: rect["height"] ?? 0),
        actions: (body["actions"] as? [[String: Any]] ?? []).compactMap(Self.action))
    }
    return true
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

  func contextMenuInteraction(
    _ interaction: UIContextMenuInteraction, configurationForMenuAtLocation location: CGPoint
  ) -> UIContextMenuConfiguration? {
    guard !dragging, let page, let press, !press.actions.isEmpty,
      press.rect.contains(interaction.view?.convert(location, to: page) ?? location)
    else { return nil }
    return UIContextMenuConfiguration(identifier: nil, previewProvider: nil) { [weak self] _ in
      let item = { (action: Action) in
        UIAction(
          title: action.title, image: action.symbol.flatMap { UIImage(systemName: $0) },
          attributes: action.destructive ? .destructive : []
        ) { _ in self?.pick(action, row: press.rowId) }
      }
      // Destructive actions sit apart, last, as the system's own menus keep them.
      let safe = press.actions.filter { !$0.destructive }.map(item)
      let destructive = press.actions.filter(\.destructive).map(item)
      return UIMenu(children: safe + (destructive.isEmpty ? [] : [
        UIMenu(options: .displayInline, children: destructive)
      ]))
    }
  }

  func contextMenuInteraction(
    _ interaction: UIContextMenuInteraction,
    configuration: UIContextMenuConfiguration,
    highlightPreviewForItemWithIdentifier identifier: any NSCopying
  ) -> UITargetedPreview? {
    rowPreview(interaction)
  }

  func contextMenuInteraction(
    _ interaction: UIContextMenuInteraction,
    previewForHighlightingMenuWithConfiguration configuration: UIContextMenuConfiguration
  ) -> UITargetedPreview? {
    rowPreview(interaction)
  }

  func contextMenuInteraction(
    _ interaction: UIContextMenuInteraction, willDisplayMenuFor configuration: UIContextMenuConfiguration,
    animator: (any UIContextMenuInteractionAnimating)?
  ) {
    menuUp = true
  }

  func contextMenuInteraction(
    _ interaction: UIContextMenuInteraction, willEndFor configuration: UIContextMenuConfiguration,
    animator: (any UIContextMenuInteractionAnimating)?
  ) {
    guard let animator else { return menuGone() }
    animator.addCompletion { [weak self] in self?.menuGone() }
  }

  private func menuGone() {
    menuUp = false
    if let alert = pendingConfirm {
      pendingConfirm = nil
      present(alert)
    }
  }

  // The row itself lifts, not the whole page.
  private func rowPreview(_ interaction: UIContextMenuInteraction) -> UITargetedPreview? {
    guard let page, let press, let container = interaction.view,
      let row = page.resizableSnapshotView(
        from: press.rect, afterScreenUpdates: false, withCapInsets: .zero)
    else { return nil }
    let parameters = UIPreviewParameters()
    parameters.visiblePath = UIBezierPath(
      roundedRect: CGRect(origin: .zero, size: press.rect.size), cornerRadius: 8)
    parameters.backgroundColor = page.underPageBackgroundColor
    let center = page.convert(CGPoint(x: press.rect.midX, y: press.rect.midY), to: container)
    return UITargetedPreview(
      view: row, parameters: parameters, target: UIPreviewTarget(container: container, center: center))
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
