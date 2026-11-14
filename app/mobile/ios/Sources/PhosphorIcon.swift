import SwiftUI

@available(iOS 26.0, *)
struct PhosphorIcon: View {
  let icon: CrowbarIcon
  let emphasis: CGFloat
  @ScaledMetric private var side: CGFloat

  init(_ icon: CrowbarIcon, size: CGFloat = 20, emphasis: CGFloat = 0) {
    self.icon = icon
    self.emphasis = emphasis
    _side = ScaledMetric(wrappedValue: size, relativeTo: .body)
  }

  var body: some View {
    let art = icon.art
    let outline = icon.path(in: side)
    Group {
      if art.stroke > 0 {
        outline.stroke(
          style: StrokeStyle(
            lineWidth: art.stroke * side / art.viewBox,
            lineCap: .round,
            lineJoin: .round
          )
        )
      } else {
        outline.fill().overlay { if emphasis > 0 { outline.stroke(lineWidth: emphasis) } }
      }
    }
    .frame(width: side, height: side)
    .accessibilityHidden(true)
  }
}
