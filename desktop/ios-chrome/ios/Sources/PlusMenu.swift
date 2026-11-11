import SwiftUI

struct PlusMenuItem: Decodable, Identifiable, Equatable {
  let id: String
  let title: String
  /// A Phosphor name.
  let icon: String
  /// The character the row types into the composer.
  let hint: String?
  /// Starts the group after a divider.
  let divider: Bool
}

/// The composer's plus menu, as `set_plus_menu` hands it over.
struct PlusMenuState: Decodable, Equatable {
  let open: Bool
  let title: String
  let items: [PlusMenuItem]

  /// The rows as list sections, split where the web puts a divider.
  var groups: [[PlusMenuItem]] {
    items.reduce(into: [[PlusMenuItem]]()) { groups, item in
      if item.divider || groups.isEmpty { groups.append([item]) } else { groups[groups.count - 1].append(item) }
    }
  }
}

final class PlusMenuModel: ObservableObject {
  @Published var state = PlusMenuState(open: false, title: "", items: [])
}

/// The plus menu as a native sheet in the web palette; a row reports its id after the sheet is gone.
@available(iOS 26.0, *)
struct PlusMenuSheet: View {
  @ObservedObject var model: PlusMenuModel
  @ObservedObject var chrome: ChromeModel
  let pick: (String) -> Void
  let close: () -> Void

  private var palette: ChromePalette? { chrome.palette }
  private var text: Color { palette?.text ?? .primary }
  private var secondaryText: Color { palette?.secondaryText ?? .secondary }

  var body: some View {
    NavigationStack {
      List {
        ForEach(Array(model.state.groups.enumerated()), id: \.offset) { _, group in
          Section {
            ForEach(group) { item in row(item) }
          }
          .listRowBackground(palette?.raised)
        }
      }
      .nativeList(palette)
      .nativeTitle(model.state.title)
      .toolbarBackgroundVisibility(.hidden, for: .navigationBar)
      .toolbar {
        ToolbarItem(placement: .topBarLeading) {
          Button(action: close) { PhosphorIcon(.x, emphasis: 0.5).foregroundStyle(text) }
            .accessibilityLabel("Close")
        }
      }
    }
  }

  private func row(_ item: PlusMenuItem) -> some View {
    Button {
      Haptics.select()
      pick(item.id)
    } label: {
      HStack(spacing: RowMetrics.gap) {
        IconRow(icon: CrowbarIcon(named: item.icon), title: item.title, text: text)
          .frame(maxWidth: .infinity, alignment: .leading)
        if let hint = item.hint {
          Text(hint).font(.system(.footnote, design: .monospaced)).foregroundStyle(secondaryText)
        }
      }
      .frame(minHeight: 44)
      .contentShape(.rect)
    }
    .buttonStyle(.plain)
  }
}
