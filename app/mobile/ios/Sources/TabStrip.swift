import SwiftUI

@available(iOS 26.0, *)
struct TabStrip: View {
  @ObservedObject var model: TabsModel
  @ObservedObject var chrome: ChromeModel

  private var ink: Color { chrome.palette?.text ?? .primary }
  private var muted: Color { chrome.palette?.secondaryText ?? .secondary }

  var body: some View {
    ScrollViewReader { scroller in
      Strip(model: model, ink: ink, muted: muted, scroller: scroller)
    }
    .dynamicTypeSize(...DynamicTypeSize.xxLarge)
  }

  private struct Strip: View {
    @ObservedObject var model: TabsModel
    let ink: Color
    let muted: Color
    let scroller: ScrollViewProxy

    private var activeId: String? { model.state.tabs.first(where: \.active)?.id }

    var body: some View {
      ScrollView(.horizontal, showsIndicators: false) { row }
        .clipShape(.capsule)
        .glassEffect(.regular, in: .capsule)
        .onChange(of: activeId, initial: true, reveal)
    }

    private var reveal: (String?, String?) -> Void {
      { _, active in
        guard let active else { return }
        withAnimation(.snappy) { scroller.scrollTo(active, anchor: .center) }
      }
    }

    private var row: some View {
      HStack(spacing: 2) {
        tabs
        NewTabMenu(model: model, ink: ink)
      }
      .padding(.horizontal, Spacing.tight)
      .frame(height: Metric.tapTarget)
    }

    private var tabs: some View {
      ForEach(model.state.tabs) { tab in
        TabPill(tab: tab, ink: ink, muted: muted, send: model.send).id(tab.id)
      }
    }
  }

  private struct NewTabMenu: View {
    @ObservedObject var model: TabsModel
    let ink: Color

    var body: some View {
      Menu {
        actions
      } label: {
        plus
      }
      .buttonStyle(.plain)
      .accessibilityLabel("New tab")
    }

    private var actions: some View {
      ForEach(model.state.newTabActions) { action in
        NewTabButton(action: action, send: model.send)
      }
    }

    private var plus: some View {
      PhosphorIcon(.plus, size: IconSize.compact, emphasis: 0.5)
        .foregroundStyle(ink)
        .frame(width: Metric.stripButton, height: Metric.tapTarget)
        .contentShape(.rect)
    }
  }

  private struct NewTabButton: View {
    let action: TabsState.NewTabAction
    let send: (ChromeIntent) -> Void

    var body: some View {
      Button(action: create) { label }
    }

    private var create: () -> Void {
      {
        Haptics.tap()
        send(.of("newTab", ["actionId": action.id]))
      }
    }

    private var label: some View {
      Label {
        Text(action.title)
      } icon: {
        CrowbarIcon(named: action.icon).map { Image(uiImage: $0.image(size: IconSize.large)) }
      }
    }
  }
}
