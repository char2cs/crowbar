import SwiftUI

private let hostLabelGap: CGFloat = 2

@available(iOS 26.0, *)
struct HostLine<Trailing: View>: View {
  let host: PickerHost
  let text: Color
  let secondaryText: Color
  var palette: ChromePalette?
  @ViewBuilder let trailing: () -> Trailing

  var body: some View {
    HStack(spacing: RowMetrics.gap) {
      HostPlaceIcon(hostId: host.id, state: host.state, palette: palette)
        .frame(width: RowMetrics.icon)
      labels
      Spacer(minLength: Spacing.small)
      trailing().frame(width: RowMetrics.trailing)
    }
  }

  private var labels: some View {
    VStack(alignment: .leading, spacing: hostLabelGap) {
      Text(host.name).foregroundStyle(text).lineLimit(1)
      Text(host.endpoint).crowbarUI(.footnote).foregroundStyle(secondaryText).lineLimit(1)
    }
  }
}
