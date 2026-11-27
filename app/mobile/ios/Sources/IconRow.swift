import SwiftUI

@available(iOS 26.0, *)
struct IconRow: View {
  let icon: CrowbarIcon?
  let title: String
  let text: Color
  var value: String?
  var secondaryText: Color = .secondary

  var body: some View {
    HStack(spacing: RowMetrics.gap) {
      glyph.frame(width: RowMetrics.icon)
      Text(title).lineLimit(1)
      valueText
    }
    .foregroundStyle(text)
  }

  private var glyph: some View {
    Group { if let icon { PhosphorIcon(icon) } }
  }

  @ViewBuilder private var valueText: some View {
    if let value {
      Spacer(minLength: Spacing.small)
      Text(value).foregroundStyle(secondaryText).lineLimit(1)
    }
  }
}
