import SwiftUI

@available(iOS 26.0, *)
struct HostPickerSheet: View {
  @ObservedObject var model: HostPickerModel
  @ObservedObject var chrome: ChromeModel

  private static let closeEmphasis: CGFloat = 0.5

  private var palette: ChromePalette? { chrome.palette }

  var body: some View {
    NavigationStack { list }
  }

  private var list: some View {
    List { HostListSection(list: model.state.list, palette: palette, send: model.send) }
      .nativeList(palette)
      .nativeTitle("Hosts")
      .toolbarBackgroundVisibility(.hidden, for: .navigationBar)
      .toolbar { toolbarContent }
  }

  @ToolbarContentBuilder private var toolbarContent: some ToolbarContent {
    ToolbarItem(placement: .topBarLeading) { closeButton }
  }

  private var closeButton: some View {
    Button(
      action: { model.send(.of("closeHostPicker")) },
      label: {
        PhosphorIcon(.xMark, emphasis: Self.closeEmphasis).foregroundStyle(palette?.text ?? .primary)
      }
    )
    .accessibilityLabel("Close")
  }
}
