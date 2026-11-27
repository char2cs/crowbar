import UIKit
import WebKit

final class StripCatcher: UIView {
  var close: () -> Void = {}
  let band = UIView()

  override init(frame: CGRect) {
    super.init(frame: frame)
    band.isUserInteractionEnabled = false
    addSubview(band)
  }

  required init?(coder: NSCoder) { return nil }

  override func accessibilityActivate() -> Bool {
    close()
    return true
  }

  override func accessibilityPerformEscape() -> Bool {
    close()
    return true
  }
}
