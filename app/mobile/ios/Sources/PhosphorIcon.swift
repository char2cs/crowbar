import SwiftUI

@available(iOS 26.0, *)
struct PhosphorIcon: View {
  let icon: CrowbarIcon
  let emphasis: CGFloat
  @ScaledMetric private var side: CGFloat

  init(
    _ icon: CrowbarIcon,
    size: CGFloat = IconSize.large,
    emphasis: CGFloat = 0
  ) {
    self.icon = icon
    self.emphasis = emphasis
    _side = ScaledMetric(wrappedValue: size, relativeTo: .body)
  }

  var body: some View {
    Group { drawing }
      .frame(width: side, height: side)
      .accessibilityHidden(true)
  }

  private var art: CrowbarIconArt { icon.art }
  private var outline: Path { icon.path(in: side) }

  @ViewBuilder private var drawing: some View {
    switch art.stroke > 0 {
    case true: outline.stroke(style: strokeStyle)
    case false: outline.fill().overlay { edge }
    }
  }

  private var strokeStyle: StrokeStyle {
    StrokeStyle(lineWidth: art.stroke * side / art.viewBox, lineCap: .round, lineJoin: .round)
  }

  @ViewBuilder private var edge: some View {
    if emphasis > 0 { outline.stroke(lineWidth: emphasis) }
  }
}
