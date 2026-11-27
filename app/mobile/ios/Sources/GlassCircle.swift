import SwiftUI

@available(iOS 26.0, *)
struct GlassCircle: View {
  let icon: String
  let label: String
  var enabled = true
  var tint: Color?
  let action: () -> Void

  static let edgeInset: CGFloat = 16
  static let diameter: CGFloat = 44
  private static let glyphEmphasis: CGFloat = 0.5

  var body: some View {
    Button(action: tap) { face }
      .buttonStyle(.plain)
      .disabled(!enabled)
      .opacity(enabled ? 1 : Alpha.muted)
      .accessibilityLabel(label)
      .dynamicTypeSize(...DynamicTypeSize.xxxLarge)
  }

  private var tap: () -> Void {
    {
      Haptics.tap()
      action()
    }
  }

  private var face: some View {
    Group { glyph }
      .foregroundStyle(tint ?? .primary)
      .frame(width: Self.diameter, height: Self.diameter)
      .contentShape(.circle)
      .glassEffect(.regular.interactive(), in: .circle)
  }

  @ViewBuilder private var glyph: some View {
    if let icon = CrowbarIcon(named: icon) { PhosphorIcon(icon, emphasis: Self.glyphEmphasis) }
  }
}
