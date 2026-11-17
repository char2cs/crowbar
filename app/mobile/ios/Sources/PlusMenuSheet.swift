import SwiftUI

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
    NavigationStack { list }
  }

  private var list: some View {
    List { groups }
      .nativeList(palette)
      .nativeTitle(model.state.title)
      .toolbarBackgroundVisibility(.hidden, for: .navigationBar)
      .toolbar { toolbarContent }
  }

  private var groups: some View {
    ForEach(Array(model.state.groups.enumerated()), id: \.offset) { _, group in
      Section { Rows(group: group, text: text, secondaryText: secondaryText, pick: pick) }
        .listRowBackground(palette?.raised)
    }
  }

  @ToolbarContentBuilder private var toolbarContent: some ToolbarContent {
    ToolbarItem(placement: .topBarLeading) { closeButton }
  }

  private var closeButton: some View {
    Button(action: close) { PhosphorIcon(.x, emphasis: 0.5).foregroundStyle(text) }
      .accessibilityLabel("Close")
  }

  private struct Rows: View {
    let group: [PlusMenuItem]
    let text: Color
    let secondaryText: Color
    let pick: (String) -> Void

    var body: some View {
      ForEach(group) { item in
        Row(item: item, text: text, secondaryText: secondaryText, pick: pick)
      }
    }
  }

  private struct Row: View {
    let item: PlusMenuItem
    let text: Color
    let secondaryText: Color
    let pick: (String) -> Void

    var body: some View {
      Button(action: choose) { label }
        .buttonStyle(.plain)
        // PLATFORM: ios: The row's own 44pt is the whole height: the list's default insets would add 27pt to it.
        .listRowInsets(
          EdgeInsets(top: 2, leading: Spacing.large, bottom: 2, trailing: Spacing.large)
        )
    }

    private var choose: () -> Void {
      {
        Haptics.select()
        pick(item.id)
      }
    }

    private var label: some View {
      HStack(spacing: RowMetrics.gap) {
        IconRow(icon: CrowbarIcon(named: item.icon), title: item.title, text: text)
          .frame(maxWidth: .infinity, alignment: .leading)
        hint
      }
      .frame(minHeight: Metric.tapTarget)
      .contentShape(.rect)
    }

    @ViewBuilder private var hint: some View {
      if let hint = item.hint {
        Text(hint).font(.system(.footnote, design: .monospaced)).foregroundStyle(secondaryText)
      }
    }
  }
}
