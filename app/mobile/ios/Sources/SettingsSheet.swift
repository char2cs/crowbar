import SwiftUI
import WebKit

@available(iOS 26.0, *)
struct SettingsSheet: View {
  @ObservedObject var model: SettingsModel
  @ObservedObject var chrome: ChromeModel
  let page: WKWebView
  let close: () -> Void
  @State private var path: [SettingsRoute] = []

  private var palette: ChromePalette? { chrome.palette }
  private var text: Color { palette?.text ?? .primary }
  private var secondaryText: Color { palette?.secondaryText ?? .secondary }

  var body: some View {
    NavigationStack(path: $path) {
      root.navigationDestination(for: SettingsRoute.self, destination: destination)
    }
  }

  private var destination: (SettingsRoute) -> Destination {
    { route in
      Destination(route: route, model: model, page: page, palette: palette, text: text)
    }
  }

  private var pages: [SettingsPageItem] { (model.state?.pages ?? []).filter { $0.id != "hosts" } }

  private struct SectionTitle: View {
    let title: String
    let color: Color

    var body: some View {
      Text(title).crowbarHeading(size: Metric.sectionTitle, relativeTo: .subheadline)
        .foregroundStyle(color)
    }
  }

  private struct Disclosure: View {
    let secondaryText: Color

    var body: some View {
      PhosphorIcon(.caretRight, size: IconSize.small).foregroundStyle(secondaryText)
        .frame(width: RowMetrics.trailing)
    }
  }

  private struct PushRow<Content: View>: View {
    let route: SettingsRoute
    var caret = true
    let secondaryText: Color
    @Binding var path: [SettingsRoute]
    @ViewBuilder let content: () -> Content

    var body: some View {
      Button(action: push) { label }
        .buttonStyle(.plain)
    }

    private var push: () -> Void {
      {
        Haptics.select()
        path.append(route)
      }
    }

    private var label: some View {
      HStack(spacing: RowMetrics.gap) {
        content().frame(maxWidth: .infinity, alignment: .leading)
        trailing
      }
      .contentShape(.rect)
    }

    @ViewBuilder private var trailing: some View {
      if caret { Disclosure(secondaryText: secondaryText) }
    }
  }

  private struct PageRow: View {
    let item: SettingsPageItem
    let text: Color
    let secondaryText: Color
    @Binding var path: [SettingsRoute]

    var body: some View {
      PushRow(
        route: .page(id: item.id, label: item.label),
        secondaryText: secondaryText,
        path: $path
      ) {
        IconRow(icon: CrowbarIcon(named: item.icon), title: item.label, text: text)
      }
    }
  }

  private var root: some View {
    List {
      hostsSection
      appearanceSection
      appSection
    }
    .nativeList(palette)
    .nativeTitle("Settings")
    .toolbarBackgroundVisibility(.hidden, for: .navigationBar)
    .toolbar { toolbarContent }
  }

  @ToolbarContentBuilder private var toolbarContent: some ToolbarContent {
    ToolbarItem(placement: .topBarLeading) { closeButton }
  }

  private var closeButton: some View {
    Button(action: close) { PhosphorIcon(.x, emphasis: 0.5).foregroundStyle(text) }
      .accessibilityLabel("Close")
  }

  private var hostsSection: some View {
    Section {
      hostsPush
    } header: {
      SectionTitle(title: hostsTitle, color: secondaryText)
    }
  }

  private var hostsPush: some View {
    PushRow(route: .hosts, caret: false, secondaryText: secondaryText, path: $path) {
      hostsRow
    }
  }

  private var appearanceSection: some View {
    Section {
      appearanceCards
    } header: {
      SectionTitle(title: "Appearance", color: secondaryText)
    }
  }

  private var appearanceCards: some View {
    AppearanceCards(mode: model.themeMode, palette: palette, choose: pickMode)
      .listRowBackground(Color.clear)
      .listRowInsets(cardInsets)
  }

  private var pickMode: (String) -> Void {
    { mode in
      Haptics.select()
      model.send(.of("setThemeMode", ["mode": mode]))
    }
  }

  private var cardInsets: EdgeInsets {
    EdgeInsets(
      top: Spacing.tight,
      leading: Spacing.large,
      bottom: Spacing.tight,
      trailing: Spacing.large
    )
  }

  private var appSection: some View {
    Section {
      themePush
      pageRows
    } header: {
      SectionTitle(title: "App", color: secondaryText)
    }
  }

  private var themePush: some View {
    PushRow(route: .theme, secondaryText: secondaryText, path: $path) {
      IconRow(
        icon: .palette,
        title: "Theme",
        text: text,
        value: themeName,
        secondaryText: secondaryText
      )
    }
  }

  private var pageRows: some View {
    ForEach(pages) { item in
      PageRow(item: item, text: text, secondaryText: secondaryText, path: $path)
    }
  }

  private var shownHost: PickerHost? {
    model.hosts.hosts.first { $0.id == model.hosts.shownHost } ?? model.hosts.hosts.first
  }

  @ViewBuilder private var hostsRow: some View {
    switch shownHost {
    case .some(let shown):
      ShownHostRow(host: shown, text: text, secondaryText: secondaryText, palette: palette)
    case .none:
      AddHostRow(text: text, secondaryText: secondaryText)
    }
  }

  private struct ShownHostRow: View {
    let host: PickerHost
    let text: Color
    let secondaryText: Color
    let palette: ChromePalette?

    var body: some View {
      HostLine(host: host, text: text, secondaryText: secondaryText, palette: palette) {
        PhosphorIcon(.caretRight, size: IconSize.small).foregroundStyle(secondaryText)
      }
    }
  }

  private struct AddHostRow: View {
    let text: Color
    let secondaryText: Color

    var body: some View {
      HStack(spacing: RowMetrics.gap) {
        IconRow(icon: .cloud, title: "Add a host", text: text)
          .frame(maxWidth: .infinity, alignment: .leading)
        Disclosure(secondaryText: secondaryText)
      }
    }
  }

  private var hostsTitle: String {
    model.state?.pages.first { $0.id == "hosts" }?.label ?? "Hosts"
  }

  private var themeName: String {
    model.state?.themes.first { $0.id == model.state?.theme }?.name ?? model.state?.theme ?? ""
  }

  private struct Destination: View {
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

  private struct PageHost: View {
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

  private struct ThemeRow: View {
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
