import SwiftUI
import UIKit

@available(iOS 26.0, *)
extension ChromeOverlays {
  private static let halfFactor: CGFloat = 0.5

  func applyJumpToLatest(_ state: JumpToLatestState) {
    guard let view = jumpButton else { return }
    if state.visible { model.jumpOwner = state.owner }
    if !state.visible && state.owner != model.jumpOwner { return }
    guard state.visible else { return hideJumpButton(view) }
    model.jumpLabel = state.label
    jumpBottom = CGFloat(state.bottom)
    showJumpButton(view)
  }

  func placeJumpButton() {
    guard let view = jumpButton, let webview else { return }
    view.center = CGPoint(
      x: webview.bounds.midX,
      y: webview.bounds.height - jumpBottom - GlassCircle.diameter * Self.halfFactor
    )
  }

  func makeJumpButton() -> UIView? {
    guard let webview else { return nil }
    let controller = UIHostingController(
      rootView: JumpToLatestButton(model: model)
    )
    controller.view.backgroundColor = .clear
    let size = GlassCircle.diameter
    controller.view.frame = CGRect(x: 0, y: 0, width: size, height: size)
    controller.view.alpha = 0
    controller.view.isHidden = true
    webview.addSubview(controller.view)
    hosted.append(controller)
    return controller.view
  }

  private func showJumpButton(_ view: UIView) {
    if view.isHidden || view.alpha == 0 { prepareJumpButton(view) }
    UIView.animate(
      withDuration: Metric.fastDuration,
      delay: 0,
      options: [.curveEaseOut, .beginFromCurrentState]
    ) {
      self.placeJumpButton()
      view.alpha = 1
      view.transform = .identity
    }
  }

  private func prepareJumpButton(_ view: UIView) {
    placeJumpButton()
    view.transform = CGAffineTransform(scaleX: Metric.popScale, y: Metric.popScale)
    view.isHidden = false
  }

  private func hideJumpButton(_ view: UIView) {
    UIView.animate(
      withDuration: Metric.fastDuration,
      delay: 0,
      options: [.curveEaseIn, .beginFromCurrentState],
      animations: {
        view.alpha = 0
        view.transform = CGAffineTransform(scaleX: Metric.popScale, y: Metric.popScale)
      },
      completion: { _ in view.isHidden = view.alpha == 0 }
    )
  }
}
