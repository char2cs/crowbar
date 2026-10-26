import SwiftUI
import UIKit

/// What a sheet should do next, from the web's `open` and what UIKit is showing. The web's value
/// is mirrored, never overwritten here, so a user swipe-down is not undone by the next state push
/// that still says open; the sheet is only presented again once the web has closed and reopened.
struct SheetPlan {
  enum Step { case none, present, dismiss }

  private var wantOpen = false
  private(set) var presented = false
  // A present or dismiss still animating: the web can flip open and closed faster than that
  // (React's dev double-mount does), so the next step waits for this one's completion.
  private(set) var moving = false

  /// The web's current `open`; acts only when it changed.
  mutating func update(open: Bool) -> Step {
    guard open != wantOpen else { return .none }
    wantOpen = open
    return next()
  }

  mutating func finished() -> Step {
    moving = false
    return next()
  }

  /// The user closed it; the web is told and answers with `open: false`.
  mutating func dismissedByUser() { presented = false }

  private mutating func next() -> Step {
    guard !moving, wantOpen != presented else { return .none }
    moving = true
    presented = wantOpen
    return wantOpen ? .present : .dismiss
  }
}

/// Presents a sheet while the web says it is open. A swipe-down is reported through
/// `onUserDismiss`; closing it from the web is not reported back.
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

  /// `undimmed`: what is above a half-height sheet stays lit and touchable.
  init(
    undimmed: Bool, detents: [UISheetPresentationController.Detent] = [.medium(), .large()],
    content: @escaping () -> UIViewController, onUserDismiss: @escaping () -> Void
  ) {
    self.undimmed = undimmed
    self.detents = detents
    makeContent = content
    self.onUserDismiss = onUserDismiss
  }

  func update(open: Bool, busy: Bool, over root: UIViewController?) {
    self.root = root ?? self.root
    sheet?.isModalInPresentation = busy
    self.busy = busy
    run(plan.update(open: open))
  }

  private func run(_ step: SheetPlan.Step) {
    switch step {
    case .none:
      break
    case .present:
      guard let root else {
        plan.dismissedByUser()
        return run(plan.finished())
      }
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
