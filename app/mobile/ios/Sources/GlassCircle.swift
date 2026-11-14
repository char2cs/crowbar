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
    .dynamicTypeSize(...DynamicTypeSize.xxxLarge)
  }
}
