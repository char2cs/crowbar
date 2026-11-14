import SwiftUI

@available(iOS 26.0, *)
struct TabStrip: View {
  @ObservedObject var model: TabsModel
  @ObservedObject var chrome: ChromeModel

  private var ink: Color { chrome.palette?.text ?? .primary }
  private var muted: Color { chrome.palette?.secondaryText ?? .secondary }

  var body: some View {
    ScrollViewReader { scroller in
      ScrollView(.horizontal, showsIndicators: false) {
        HStack(spacing: 2) {
          ForEach(model.state.tabs) { tab in
            TabPill(tab: tab, ink: ink, muted: muted, send: model.send).id(tab.id)
          }
          newTab
        }
        .padding(.horizontal, 4)
        .frame(height: 44)
      }
      .clipShape(.capsule)
      .glassEffect(.regular, in: .capsule)
      .onChange(of: model.state.tabs.first(where: \.active)?.id, initial: true) { _, active in
        guard let active else { return }
        withAnimation(.snappy) { scroller.scrollTo(active, anchor: .center) }
      }
    }
    .dynamicTypeSize(...DynamicTypeSize.xxLarge)
  }

  private var newTab: some View {
    Menu {
      ForEach(model.state.newTabActions) { action in
        Button {
          Haptics.tap()
          model.send(.of("newTab", ["actionId": action.id]))
        } label: {
          Label {
            Text(action.title)
          } icon: {
            CrowbarIcon(named: action.icon).map { Image(uiImage: $0.image(size: 20)) }
          }
        }
      }
    } label: {
      PhosphorIcon(.plus, size: 16, emphasis: 0.5)
        .foregroundStyle(ink)
        .frame(width: 32, height: 44)
        .contentShape(.rect)
    }
    .buttonStyle(.plain)
    .accessibilityLabel("New tab")
  }
}
