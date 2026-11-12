import SwiftUI

/// The chrome's round 44pt glass button.
@available(iOS 26.0, *)
struct GlassCircle: View {
  /// The web's name for a Phosphor icon.
  let icon: String
  let label: String
  var enabled = true
  /// The theme's foreground: the glass is clear, so the glyph must read on the page behind it.
  var tint: Color?
  let action: () -> Void

  /// From the screen edge to the circle.
  static let edgeInset: CGFloat = 16
  static let diameter: CGFloat = 44

  var body: some View {
    Button {
      Haptics.tap()
      action()
    } label: {
      Group {
        if let icon = CrowbarIcon(named: icon) { PhosphorIcon(icon, emphasis: 0.5) }
      }
      .foregroundStyle(tint ?? .primary)
      .frame(width: Self.diameter, height: Self.diameter)
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

/// The transcript's way back to the newest rows: a neutral down arrow.
@available(iOS 26.0, *)
struct JumpToLatestButton: View {
  @ObservedObject var model: ChromeModel

  var body: some View {
    GlassCircle(
      icon: "ArrowDown", label: model.jumpLabel,
      tint: model.palette?.text
    ) {
      model.send(.of("jumpToLatest"))
    }
  }
}
