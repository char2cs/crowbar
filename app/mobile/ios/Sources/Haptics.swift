import UIKit

enum Haptics {
  static func tap() {
    UIImpactFeedbackGenerator(style: .light).impactOccurred()
  }

  static func lift() {
    UIImpactFeedbackGenerator(style: .medium).impactOccurred()
  }

  static func select() {
    UISelectionFeedbackGenerator().selectionChanged()
  }
}
