import SwiftUI

@available(iOS 26.0, *)
struct HostPickerSheet: View {
  @ObservedObject var model: HostPickerModel
  @ObservedObject var chrome: ChromeModel

  private var palette: ChromePalette? { chrome.palette }

  var body: some View {
    NavigationStack {
      List { HostListSection(list: model.state.list, palette: palette, send: model.send) }
        .nativeList(palette)
        .nativeTitle("Hosts")
        .toolbarBackgroundVisibility(.hidden, for: .navigationBar)
        .toolbar {
          ToolbarItem(placement: .topBarLeading) {
            Button {
              model.send(.of("closeHostPicker"))
            } label: {
              PhosphorIcon(.x, emphasis: 0.5).foregroundStyle(palette?.text ?? .primary)
            }
            .accessibilityLabel("Close")
          }
        }
    }
  }
}
