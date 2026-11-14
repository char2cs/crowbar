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
      root
        .navigationDestination(for: SettingsRoute.self) { route in
          Destination(route: route, model: model, page: page, palette: palette, text: text)
        }
    }
  }

  private var pages: [SettingsPageItem] { (model.state?.pages ?? []).filter { $0.id != "hosts" } }

  private struct PushRow<Content: View>: View {
    let route: SettingsRoute
    var caret = true
    let secondaryText: Color
    @Binding var path: [SettingsRoute]
    @ViewBuilder let content: () -> Content

    var body: some View {
      Button {
        Haptics.select()
        path.append(route)
      } label: {
        HStack(spacing: RowMetrics.gap) {
          content().frame(maxWidth: .infinity, alignment: .leading)
          if caret {
            PhosphorIcon(.caretRight, size: 14).foregroundStyle(secondaryText)
              .frame(width: RowMetrics.trailing)
          }
        }
        .contentShape(.rect)
      }
      .buttonStyle(.plain)
    }
  }

  private var disclosure: some View {
    PhosphorIcon(.caretRight, size: 14).foregroundStyle(secondaryText)
      .frame(width: RowMetrics.trailing)
  }

  private var root: some View {
    List {
      Section {
        PushRow(route: .hosts, caret: false, secondaryText: secondaryText, path: $path) { hostsRow }
      } header: {
        Text(hostsTitle).crowbarHeading(size: 17, relativeTo: .subheadline)
          .foregroundStyle(secondaryText)
      }
      Section {
        AppearanceCards(mode: model.themeMode, palette: palette) { mode in
          Haptics.select()
          model.send(.of("setThemeMode", ["mode": mode]))
        }
        .listRowBackground(Color.clear)
        .listRowInsets(EdgeInsets(top: 4, leading: 16, bottom: 4, trailing: 16))
      } header: {
        Text("Appearance").crowbarHeading(size: 17, relativeTo: .subheadline)
          .foregroundStyle(secondaryText)
      }
      Section {
        PushRow(route: .theme, secondaryText: secondaryText, path: $path) {
          IconRow(
            icon: .palette,
            title: "Theme",
            text: text,
            value: themeName,
            secondaryText: secondaryText
          )
        }
        ForEach(pages) { item in
          PushRow(
            route: .page(id: item.id, label: item.label),
            secondaryText: secondaryText,
            path: $path
          ) {
            IconRow(icon: CrowbarIcon(named: item.icon), title: item.label, text: text)
          }
        }
      } header: {
        Text("App").crowbarHeading(size: 17, relativeTo: .subheadline)
          .foregroundStyle(secondaryText)
      }
    }
    .nativeList(palette)
    .nativeTitle("Settings")
    .toolbarBackgroundVisibility(.hidden, for: .navigationBar)
    .toolbar {
      ToolbarItem(placement: .topBarLeading) {
        Button(action: close) { PhosphorIcon(.x, emphasis: 0.5).foregroundStyle(text) }
          .accessibilityLabel("Close")
      }
    }
  }

  @ViewBuilder private var hostsRow: some View {
    if let shown = model.hosts.hosts.first(where: { $0.id == model.hosts.shownHost })
      ?? model.hosts.hosts.first
    {
      HostLine(host: shown, text: text, secondaryText: secondaryText, palette: palette) {
        PhosphorIcon(.caretRight, size: 14).foregroundStyle(secondaryText)
      }
    } else {
      HStack(spacing: RowMetrics.gap) {
        IconRow(icon: .cloud, title: "Add a host", text: text)
          .frame(maxWidth: .infinity, alignment: .leading)
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

  private struct Destination: View {
    let route: SettingsRoute
    @ObservedObject var model: SettingsModel
    let page: WKWebView
    let palette: ChromePalette?
    let text: Color

    var body: some View {
      switch route {
      case .hosts:
        List { HostListSection(list: model.hosts, palette: palette, send: model.send) }
          .nativeList(palette)
          .nativeTitle("Hosts")
      case .theme:
        List {
          Section {
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
  }

  private struct ThemeRow: View {
    let theme: ThemeChoice
    let isCurrent: Bool
    let text: Color
    let send: (ChromeIntent) -> Void

    var body: some View {
      Button {
        Haptics.select()
        send(.of("setTheme", ["id": theme.id]))
      } label: {
        HStack {
          Text(theme.name).foregroundStyle(text)
          Spacer()
          if isCurrent {
            PhosphorIcon(.check, size: 18).frame(width: RowMetrics.trailing)
          }
        }
        .contentShape(.rect)
      }
      .tint(text)
      .accessibilityAddTraits(isCurrent ? .isSelected : [])
    }
  }
}
