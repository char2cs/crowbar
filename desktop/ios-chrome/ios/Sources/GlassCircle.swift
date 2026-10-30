import SwiftUI

/// The chrome's round 44pt glass button.
@available(iOS 26.0, *)
struct GlassCircle: View {
  /// The web's name for a Phosphor icon.
  let icon: String
  let label: String
  var enabled = true
  let action: () -> Void

  /// From the screen edge to the circle.
  static let edgeInset: CGFloat = 16

  var body: some View {
    Button {
      Haptics.tap()
      action()
    } label: {
      Group {
        if let icon = CrowbarIcon(named: icon) { PhosphorIcon(icon) }
      }
      .frame(width: 44, height: 44)
        .contentShape(.circle)
        .glassEffect(.regular.interactive(), in: .circle)
    }
    .buttonStyle(.plain)
    .disabled(!enabled)
    .opacity(enabled ? 1 : 0.4)
    .accessibilityLabel(label)
    // Glyphs grow with the text size, up to what still fits the fixed circle.
    .dynamicTypeSize(...DynamicTypeSize.xxxLarge)
  }
}
