import SwiftUI
import WebKit

/// Settings as a native navigation stack: a grouped list whose rows push native screens (hosts,
/// theme) or the web's own settings tabs, hosted in the one warm settings page. Colours are the
/// web palette's, so it reads as the app's own pages.
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
      root
        .navigationDestination(for: SettingsRoute.self) { route in screen(route) }
    }
  }

  private var pages: [SettingsPageItem] { (model.state?.pages ?? []).filter { $0.id != "hosts" } }

  /// A row that pushes `route`, with the app's own caret: the list's automatic disclosure
  /// chevron is a system symbol, so rows push by hand instead of through a NavigationLink.
  private func pushRow<Content: View>(
    _ route: SettingsRoute, caret: Bool = true, @ViewBuilder content: () -> Content
  ) -> some View {
    Button {
      Haptics.select()
      path.append(route)
    } label: {
      HStack(spacing: RowMetrics.gap) {
        content().frame(maxWidth: .infinity, alignment: .leading)
        if caret { disclosure }
      }
      .contentShape(.rect)
    }
    .buttonStyle(.plain)
  }

  /// In the trailing column every row shares, so each row's caret sits at the same x.
  private var disclosure: some View {
    PhosphorIcon(.caretRight, size: 14).foregroundStyle(secondaryText)
      .frame(width: RowMetrics.trailing)
  }

  private var root: some View {
    List {
      Section {
        pushRow(.hosts, caret: false) { hostsRow }
      } header: {
        Text(hostsTitle).crowbarHeading(size: 17, relativeTo: .subheadline).foregroundStyle(secondaryText)
      }
      Section {
        AppearanceCards(mode: model.themeMode, palette: palette) { mode in
          Haptics.select()
          model.send(.of("setThemeMode", ["mode": mode]))
        }
        .listRowBackground(Color.clear)
        .listRowInsets(EdgeInsets(top: 4, leading: 16, bottom: 4, trailing: 16))
      } header: {
        Text("Appearance").crowbarHeading(size: 17, relativeTo: .subheadline).foregroundStyle(secondaryText)
      }
      Section {
        pushRow(.theme) {
          IconRow(
            icon: .palette, title: "Theme", text: text, value: themeName,
            secondaryText: secondaryText)
        }
        ForEach(pages) { item in
          pushRow(.page(id: item.id, label: item.label)) {
            IconRow(icon: CrowbarIcon(named: item.icon), title: item.label, text: text)
          }
        }
      } header: {
        Text("App").crowbarHeading(size: 17, relativeTo: .subheadline).foregroundStyle(secondaryText)
      }
    }
    .nativeList(palette)
    .nativeTitle("Settings")
    .toolbarBackgroundVisibility(.hidden, for: .navigationBar)
    .toolbar {
      ToolbarItem(placement: .topBarLeading) {
        Button(action: close) { PhosphorIcon(.x, emphasis: 0.5).foregroundStyle(text) }.accessibilityLabel("Close")
      }
    }
  }

  /// The one place hosts appear on the root: the host the chat list shows, pushing the list of
  /// them (where Add host is), or a prompt to add the first.
  @ViewBuilder private var hostsRow: some View {
    if let shown = model.hosts.hosts.first(where: { $0.id == model.hosts.shownHost })
      ?? model.hosts.hosts.first
    {
      HostLine(host: shown, text: text, secondaryText: secondaryText, palette: palette) {
        PhosphorIcon(.caretRight, size: 14).foregroundStyle(secondaryText)
      }
    } else {
      HStack(spacing: RowMetrics.gap) {
        IconRow(icon: .cloud, title: "Add a host", text: text).frame(maxWidth: .infinity, alignment: .leading)
        disclosure
      }
    }
  }

  private var hostsTitle: String {
    model.state?.pages.first { $0.id == "hosts" }?.label ?? "Hosts"
  }

  private var themeName: String {
    model.state?.themes.first { $0.id == model.state?.theme }?.name ?? model.state?.theme ?? ""
  }

  @ViewBuilder private func screen(_ route: SettingsRoute) -> some View {
    switch route {
    case .hosts:
      List { HostListSection(list: model.hosts, palette: palette, send: model.send) }
        .nativeList(palette)
        .nativeTitle("Hosts")
    case .theme:
      List {
        Section {
          ForEach(model.state?.themes ?? []) { theme in themeRow(theme) }
        }
        }
      .nativeList(palette)
      .nativeTitle("Theme")
    case .page(let id, let label):
      HostedPage(page: page)
        .background(palette?.surface.ignoresSafeArea())
        .ignoresSafeArea(edges: .bottom)
        .nativeTitle(label)
        .onAppear { model.showTab(id) }
        .onDisappear { model.showTab(nil) }
    }
  }

  private func themeRow(_ theme: ThemeChoice) -> some View {
    Button {
      Haptics.select()
      model.send(.of("setTheme", ["id": theme.id]))
    } label: {
      HStack {
        Text(theme.name).foregroundStyle(text)
        Spacer()
        if theme.id == model.state?.theme {
          PhosphorIcon(.check, size: 18).frame(width: RowMetrics.trailing)
        }
      }
      .contentShape(.rect)
    }
    .tint(text)
    .accessibilityAddTraits(theme.id == model.state?.theme ? .isSelected : [])
  }
}

/// The one warm settings page, put into a pushed screen.
private struct HostedPage: UIViewRepresentable {
  let page: WKWebView

  func makeUIView(context: Context) -> WKWebView { page }
  func updateUIView(_ view: WKWebView, context: Context) {}
}
