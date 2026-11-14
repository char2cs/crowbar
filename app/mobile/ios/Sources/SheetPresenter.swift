import SwiftUI
import UIKit

@available(iOS 26.0, *)
final class SheetPresenter: NSObject, UIAdaptivePresentationControllerDelegate {
  private let makeContent: () -> UIViewController
  private let onUserDismiss: () -> Void
  private let undimmed: Bool
  private let detents: [UISheetPresentationController.Detent]
  private var sheet: UIViewController?
  private var busy = false
  private var plan = SheetPlan()
  private weak var root: UIViewController?

  init(
    undimmed: Bool,
    detents: [UISheetPresentationController.Detent] = [.medium(), .large()],
    content: @escaping () -> UIViewController,
    onUserDismiss: @escaping () -> Void
  ) {
    self.undimmed = undimmed
    self.detents = detents
    makeContent = content
    self.onUserDismiss = onUserDismiss
  }

  func update(
    _ opening: SheetOpening,
    dismissal: SheetDismissal = .allowed,
    over root: UIViewController?
  ) {
    self.root = root ?? self.root
    let blocked = dismissal == .blocked
    sheet?.isModalInPresentation = blocked
    busy = blocked
    run(plan.update(opening))
  }

  func closeThen(_ done: @escaping () -> Void) {
    guard let open = sheet else { return done() }
    sheet = nil
    plan.dismissedByUser()
    open.dismiss(animated: true, completion: done)
  }

  private func run(_ step: SheetPlan.Step) {
    switch step {
    case .none:
      break
    case .present:
      guard let root else { return run(plan.noRoot()) }
      present(over: Dialogs.topmost(root))
    case .dismiss:
      guard let open = sheet else { return finish() }
      open.dismiss(animated: true) { [weak self] in
        self?.sheet = nil
        self?.finish()
      }
    }
  }

  private func finish() { run(plan.finished()) }

  private func present(over presenter: UIViewController) {
    let controller = makeContent()
    controller.isModalInPresentation = busy
    if let sheet = controller.sheetPresentationController {
      sheet.detents = detents
      sheet.prefersGrabberVisible = true
      if undimmed { sheet.largestUndimmedDetentIdentifier = .medium }
      sheet.prefersScrollingExpandsWhenScrolledToEdge = false
    }
    controller.presentationController?.delegate = self
    sheet = controller
    presenter.present(controller, animated: true) { [weak self] in self?.finish() }
  }

  func presentationControllerDidDismiss(_ presentationController: UIPresentationController) {
    sheet = nil
    plan.dismissedByUser()
    onUserDismiss()
  }
}
