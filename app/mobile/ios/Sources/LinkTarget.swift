import UIKit
import WebKit

@available(iOS 26.0, *)
final class LinkTarget: NSObject {
  private weak var controller: NavigationController?

  init(_ controller: NavigationController) {
    self.controller = controller
  }

  @objc func step(_ link: CADisplayLink) {
    guard let controller else { return link.invalidate() }
    controller.step(link)
  }
}
