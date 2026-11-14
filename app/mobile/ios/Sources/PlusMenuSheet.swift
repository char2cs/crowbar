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
    NavigationStack {
      List {
        ForEach(Array(model.state.groups.enumerated()), id: \.offset) { _, group in
          Section {
            ForEach(group) { item in
              Row(item: item, text: text, secondaryText: secondaryText, pick: pick)
            }
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

  private struct Row: View {
    let item: PlusMenuItem
    let text: Color
    let secondaryText: Color
    let pick: (String) -> Void

    var body: some View {
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
      // PLATFORM: The row's own 44pt is the whole height: the list's default insets would add 27pt to it.
      .listRowInsets(EdgeInsets(top: 2, leading: 16, bottom: 2, trailing: 16))
    }
  }
}
