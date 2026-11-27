import SwiftUI

@available(iOS 26.0, *)
struct TabPill: View {
  let tab: TabsState.Tab
  let ink: Color
  let muted: Color
  let send: (ChromeIntent) -> Void

  private static let closeTrailing: CGFloat = 2
  private static let closeEmphasis: CGFloat = 0.5

  var body: some View {
    HStack(spacing: 0) {
      select
      close
    }
    .background(highlight)
  }

  @ViewBuilder private var highlight: some View {
    if tab.active { Capsule().fill(.primary.opacity(Alpha.faint)).frame(height: Metric.pillHeight) }
  }

  private var select: some View {
    Button(action: choose) { label }
      .buttonStyle(.plain)
      .accessibilityLabel(tab.dirty ? "\(tab.title), edited" : tab.title)
      .accessibilityAddTraits(tab.active ? .isSelected : [])
  }

  private var choose: () -> Void {
    {
      Haptics.select()
      send(.of("selectTab", ["id": tab.id]))
    }
  }

  private var label: some View {
    HStack(spacing: Spacing.chip) {
      icon
      title
    }
    .padding(.leading, Metric.pillInset)
    .padding(.trailing, tab.closable ? Self.closeTrailing : Metric.pillInset)
    .frame(height: Metric.tapTarget)
    .contentShape(.rect)
  }

  private var icon: some View {
    PhosphorIcon(
      TabIcon.of(kind: tab.kind, path: tab.path),
      size: IconSize.small,
      emphasis: Metric.dimEmphasis
    )
    .foregroundStyle(tab.active ? ink : muted)
  }

  private var title: some View {
    Text(tab.title)
      .font(.footnote.weight(tab.active ? .semibold : .regular))
      .lineLimit(1)
      .frame(maxWidth: Metric.pillTitleMax, alignment: .leading)
      .fixedSize(horizontal: true, vertical: false)
  }

  @ViewBuilder private var close: some View {
    if tab.closable {
      Button(action: closeTab) { closeIcon }
        .buttonStyle(.plain)
        .accessibilityLabel("Close \(tab.title)")
    }
  }

  private var closeTab: () -> Void {
    {
      Haptics.tap()
      send(.of("closeTab", ["id": tab.id]))
    }
  }

  private var closeIcon: some View {
    PhosphorIcon(
      tab.dirty ? .circle : .xMark,
      size: tab.dirty ? Metric.statusDot : IconSize.dirtyCompact,
      emphasis: tab.dirty ? 0 : Self.closeEmphasis
    )
    .foregroundStyle(ink)
    .frame(width: Spacing.xwide, height: Metric.tapTarget)
    .contentShape(.rect)
  }
}
