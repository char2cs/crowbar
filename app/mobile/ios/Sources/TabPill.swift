import SwiftUI

@available(iOS 26.0, *)
struct TabPill: View {
  let tab: TabsState.Tab
  let ink: Color
  let muted: Color
  let send: (ChromeIntent) -> Void

  var body: some View {
    HStack(spacing: 0) {
      Button {
        Haptics.select()
        send(.of("selectTab", ["id": tab.id]))
      } label: {
        HStack(spacing: 5) {
          PhosphorIcon(TabIcon.of(kind: tab.kind, path: tab.path), size: 14, emphasis: 0.4)
            .foregroundStyle(tab.active ? ink : muted)
          Text(tab.title)
            .font(.footnote.weight(tab.active ? .semibold : .regular))
            .lineLimit(1)
            .frame(maxWidth: 132, alignment: .leading)
            .fixedSize(horizontal: true, vertical: false)
        }
        .padding(.leading, 10)
        .padding(.trailing, tab.closable ? 2 : 10)
        .frame(height: 44)
        .contentShape(.rect)
      }
      .buttonStyle(.plain)
      .accessibilityLabel(tab.dirty ? "\(tab.title), edited" : tab.title)
      .accessibilityAddTraits(tab.active ? .isSelected : [])
      if tab.closable { close }
    }
    .background { if tab.active { Capsule().fill(.primary.opacity(0.12)).frame(height: 34) } }
  }

  private var close: some View {
    Button {
      Haptics.tap()
      send(.of("closeTab", ["id": tab.id]))
    } label: {
      PhosphorIcon(
        tab.dirty ? .circle : .x,
        size: tab.dirty ? 7 : 11,
        emphasis: tab.dirty ? 0 : 0.5
      )
      .foregroundStyle(ink)
      .frame(width: 24, height: 44)
      .contentShape(.rect)
    }
    .buttonStyle(.plain)
    .accessibilityLabel("Close \(tab.title)")
  }
}
