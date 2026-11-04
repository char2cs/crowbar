import UIKit

/// The chrome's touch feedback: a light tap for buttons, drawer commits and drops, a firmer
/// lift for picking a row up, a selection tick for picking a value.
enum Haptics {
  static func tap() {
    UIImpactFeedbackGenerator(style: .light).impactOccurred()
  }

  /// Picking something up to carry it, as a row lifting to be reordered.
  static func lift() {
    UIImpactFeedbackGenerator(style: .medium).impactOccurred()
  }

  static func select() {
    UISelectionFeedbackGenerator().selectionChanged()
  }
}
