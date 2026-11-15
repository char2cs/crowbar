import SwiftUI

@available(iOS 26.0, *)
struct HostPlaceIcon: View {
  let hostId: String
  let state: String
  var palette: ChromePalette?
  @Environment(\.colorScheme) private var scheme

  var body: some View {
    PhosphorIcon(hostId == "local" ? .desktop : .cloud, size: IconSize.large)
      .mask { cutout }
      .overlay(alignment: .bottomTrailing) { dot.offset(x: 1, y: 1) }
      .accessibilityLabel(HostDot.words(state))
  }

  private var cutout: some View {
    Rectangle()
      .overlay(alignment: .bottomTrailing) { notch }
      .compositingGroup()
  }

  private var notch: some View {
    Circle().frame(width: 10, height: 10).offset(x: 1, y: 1).blendMode(.destinationOut)
  }

  @ViewBuilder private var dot: some View {
    switch HostDot.tone(state) {
    case .unsettled:
      Circle().strokeBorder(Color.secondary, lineWidth: 1)
        .frame(width: Metric.statusDot, height: Metric.statusDot)
    case let tone:
      Circle()
        .fill(tone == .online ? palette?.brand(scheme) ?? HostDot.color(tone) : HostDot.color(tone))
        .frame(width: Metric.statusDot, height: Metric.statusDot)
    }
  }
}
