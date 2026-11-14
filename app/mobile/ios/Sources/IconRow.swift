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
      Group { if let icon { PhosphorIcon(icon) } }.frame(width: RowMetrics.icon)
      Text(title).lineLimit(1)
      if let value {
        Spacer(minLength: 8)
        Text(value).foregroundStyle(secondaryText).lineLimit(1)
      }
    }
    .foregroundStyle(text)
  }
}
