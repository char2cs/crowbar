import SwiftUI

struct TabsState: Decodable, Equatable {
  struct Tab: Decodable, Identifiable, Equatable {
    let id: String
    let title: String
    let kind: String
    let path: String?
    let active: Bool
    let dirty: Bool
    let closable: Bool
  }
  struct NewTabAction: Decodable, Identifiable, Equatable {
    let id: String
    let title: String
    let symbol: String
  }
  let tabs: [Tab]
  let newTabActions: [NewTabAction]
}

/// What the web last handed the IDE's tab strip.
final class TabsModel: ObservableObject {
  @Published var state = TabsState(tabs: [], newTabActions: [])
  var send: (ChromeIntent) -> Void = { _ in }
}

/// The IDE's open tabs, one glass bar in the header row between Back and Files.
@available(iOS 26.0, *)
struct TabStrip: View {
  @ObservedObject var model: TabsModel

  var body: some View {
    ScrollViewReader { scroller in
      ScrollView(.horizontal, showsIndicators: false) {
        HStack(spacing: 2) {
          ForEach(model.state.tabs) { tab in
            TabPill(tab: tab, send: model.send).id(tab.id)
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
          Label(action.title, systemImage: action.symbol)
        }
      }
    } label: {
      Image(systemName: "plus")
        .font(.footnote.weight(.semibold))
        .frame(width: 32, height: 44)
        .contentShape(.rect)
    }
    .buttonStyle(.plain)
    .accessibilityLabel("New tab")
  }
}

@available(iOS 26.0, *)
private struct TabPill: View {
  let tab: TabsState.Tab
  let send: (ChromeIntent) -> Void

  var body: some View {
    HStack(spacing: 0) {
      Button {
        Haptics.select()
        send(.of("selectTab", ["id": tab.id]))
      } label: {
        HStack(spacing: 5) {
          Image(systemName: TabSymbol.of(kind: tab.kind, path: tab.path))
            .font(.caption)
            .foregroundStyle(.secondary)
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

  // An edited tab shows a dot where the × sits; either closes it, as the desktop strip does.
  private var close: some View {
    Button {
      Haptics.tap()
      send(.of("closeTab", ["id": tab.id]))
    } label: {
      Image(systemName: tab.dirty ? "circle.fill" : "xmark")
        .font(.system(size: tab.dirty ? 6 : 9, weight: .bold))
        .foregroundStyle(.secondary)
        .frame(width: 24, height: 44)
        .contentShape(.rect)
    }
    .buttonStyle(.plain)
    .accessibilityLabel("Close \(tab.title)")
  }
}

/// SF Symbols standing in for the desktop's tab icons.
enum TabSymbol {
  static func of(kind: String, path: String?) -> String {
    switch kind {
    case "terminal": "apple.terminal"
    case "review": "arrow.triangle.pull"
    case "diff": "arrow.triangle.branch"
    case "browser": "globe"
    case "preview": "eye"
    case "newTab": "plus.square.dashed"
    default: file(path)
    }
  }

  private static func file(_ path: String?) -> String {
    switch path.map({ ($0 as NSString).pathExtension.lowercased() }) {
    case "swift": "swift"
    case "md", "mdx", "txt": "doc.plaintext"
    case "json", "yaml", "yml", "toml": "curlybraces"
    case "png", "jpg", "jpeg", "gif", "svg", "webp": "photo"
    case "ts", "tsx", "js", "jsx", "go", "rs", "py", "rb", "c", "h", "cpp", "java", "kt", "css",
      "html", "sh":
      "chevron.left.forwardslash.chevron.right"
    default: "doc.text"
    }
  }
}
