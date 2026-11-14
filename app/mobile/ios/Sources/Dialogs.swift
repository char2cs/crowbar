import UIKit

/// Native questions the main webview asks (`confirm`, `prompt`), shown over whatever sheet is
/// up, so they stay visible while the sheet covers the app.
enum Dialogs {
  static func topmost(_ root: UIViewController) -> UIViewController {
    var top = root
    while let next = top.presentedViewController, !next.isBeingDismissed { top = next }
    return top
  }

  struct ConfirmOptions: Decodable {
    let title: String
    let message: String
    let confirmLabel: String
    let cancelLabel: String
    let destructive: Bool
  }

  struct PromptOptions: Decodable {
    let title: String
    let message: String?
    let value: String
    let placeholder: String
    let confirmLabel: String
  }

  static func confirm(
    _ options: ConfirmOptions, over root: UIViewController, answer: @escaping (Bool) -> Void
  ) {
    let alert = UIAlertController(
      title: options.title, message: options.message, preferredStyle: .alert)
    alert.addAction(UIAlertAction(title: options.cancelLabel, style: .cancel) { _ in answer(false) })
    alert.addAction(
      UIAlertAction(title: options.confirmLabel, style: options.destructive ? .destructive : .default)
      { _ in answer(true) })
    topmost(root).present(alert, animated: true)
  }

  static func prompt(
    _ options: PromptOptions, over root: UIViewController, answer: @escaping (String?) -> Void
  ) {
    let alert = UIAlertController(
      title: options.title, message: options.message, preferredStyle: .alert)
    alert.addTextField { field in
      field.text = options.value
      field.placeholder = options.placeholder
      field.clearButtonMode = .whileEditing
    }
    alert.addAction(UIAlertAction(title: "Cancel", style: .cancel) { _ in answer(nil) })
    alert.addAction(
      UIAlertAction(title: options.confirmLabel, style: .default) { [weak alert] _ in
        answer(alert?.textFields?.first?.text ?? "")
      })
    topmost(root).present(alert, animated: true)
  }
}
