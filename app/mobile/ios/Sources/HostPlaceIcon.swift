import SwiftUI

@available(iOS 26.0, *)
struct HostPlaceIcon: View {
  let hostId: String
  let state: String
  var palette: ChromePalette?
  @Environment(\.colorScheme) private var scheme

  var body: some View {
    PhosphorIcon(hostId == "local" ? .desktop : .cloud, size: 20)
      .mask {
        Rectangle()
          .overlay(alignment: .bottomTrailing) {
            Circle().frame(width: 10, height: 10).offset(x: 1, y: 1).blendMode(.destinationOut)
          }
          .compositingGroup()
      }
      .overlay(alignment: .bottomTrailing) { dot.offset(x: 1, y: 1) }
      .accessibilityLabel(HostDot.words(state))
  }

  @ViewBuilder private var dot: some View {
    switch HostDot.tone(state) {
    case .unsettled:
      Circle().strokeBorder(Color.secondary, lineWidth: 1).frame(width: 7, height: 7)
    case let tone:
      Circle()
        .fill(tone == .online ? palette?.brand(scheme) ?? HostDot.color(tone) : HostDot.color(tone))
        .frame(width: 7, height: 7)
    }
  }
}
