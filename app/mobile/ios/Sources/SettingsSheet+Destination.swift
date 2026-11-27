import SwiftUI
import WebKit

@available(iOS 26.0, *)
extension SettingsSheet {
  struct Destination: View {
    let route: SettingsRoute
    @ObservedObject var model: SettingsModel
    let page: WKWebView
    let palette: ChromePalette?
    let text: Color

    var body: some View {
      switch route {
      case .hosts: hosts
      case .theme: themes
      case .page(let id, let label):
        PageHost(id: id, label: label, model: model, page: page, palette: palette)
      }
    }

    private var hosts: some View {
      List { HostListSection(list: model.hosts, palette: palette, send: model.send) }
        .nativeList(palette)
        .nativeTitle("Hosts")
    }

    private var themes: some View {
      List { Section { themeRows } }
        .nativeList(palette)
        .nativeTitle("Theme")
    }

    private var themeRows: some View {
      ForEach(model.state?.themes ?? []) { theme in
        ThemeRow(
          theme: theme,
          isCurrent: theme.id == model.state?.theme,
          text: text,
          send: model.send
        )
      }
    }
  }

  struct PageHost: View {
    let id: String
    let label: String
    @ObservedObject var model: SettingsModel
    let page: WKWebView
    let palette: ChromePalette?

    var body: some View {
      HostedPage(page: page)
        .background(palette?.surface.ignoresSafeArea())
        .ignoresSafeArea(edges: .bottom)
        .nativeTitle(label)
        .onAppear { model.showTab(id) }
        .onDisappear { model.showTab(nil) }
    }
  }

  struct ThemeRow: View {
    let theme: ThemeChoice
    let isCurrent: Bool
    let text: Color
    let send: (ChromeIntent) -> Void

    var body: some View {
      Button(action: choose) { label }
        .tint(text)
        .accessibilityAddTraits(isCurrent ? .isSelected : [])
    }

    private var choose: () -> Void {
      {
        Haptics.select()
        send(.of("setTheme", ["id": theme.id]))
      }
    }

    private var label: some View {
      HStack {
        Text(theme.name).foregroundStyle(text)
        Spacer()
        mark
      }
      .contentShape(.rect)
    }

    @ViewBuilder private var mark: some View {
      if isCurrent {
        PhosphorIcon(.check, size: IconSize.standard).frame(width: RowMetrics.trailing)
      }
    }
  }
}
