import SwiftUI
import UIKit

/// Presents a sheet while the web says it is open. A swipe-down is reported through
/// `onUserDismiss`; closing it from the web is not reported back.
@available(iOS 26.0, *)
final class SheetPresenter: NSObject, UIAdaptivePresentationControllerDelegate {
  private let makeContent: () -> UIViewController
  private let onUserDismiss: () -> Void
  private let undimmed: Bool
  private let detents: [UISheetPresentationController.Detent]
  private var sheet: UIViewController?
  private var wantOpen = false
  private var busy = false
  // A present or dismiss still animating: the web can flip open and closed faster than that
  // (React's dev double-mount does), so the next step waits for this one's completion.
  private var moving = false
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
    wantOpen = open
    self.busy = busy
    self.root = root ?? self.root
    reconcile()
  }

  private func reconcile() {
    sheet?.isModalInPresentation = busy
    guard !moving else { return }
    if wantOpen, sheet == nil, let root {
      present(over: Dialogs.topmost(root))
    } else if !wantOpen, let open = sheet {
      moving = true
      open.dismiss(animated: true) { [weak self] in self?.settled(sheet: nil) }
    }
  }

  private func settled(sheet: UIViewController?) {
    self.sheet = sheet
    moving = false
    reconcile()
  }

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
    moving = true
    sheet = controller
    presenter.present(controller, animated: true) { [weak self] in
      self?.settled(sheet: controller)
    }
  }

  // The user closed it: that is the web's to act on, so it is not presented again meanwhile.
  func presentationControllerDidDismiss(_ presentationController: UIPresentationController) {
    sheet = nil
    wantOpen = false
    onUserDismiss()
  }
}
