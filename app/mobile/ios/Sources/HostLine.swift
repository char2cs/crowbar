import SwiftUI

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
      VStack(alignment: .leading, spacing: 2) {
        Text(host.name).foregroundStyle(text).lineLimit(1)
        Text(host.endpoint).crowbarUI(.footnote).foregroundStyle(secondaryText).lineLimit(1)
      }
      Spacer(minLength: 8)
      trailing().frame(width: RowMetrics.trailing)
    }
  }
}
