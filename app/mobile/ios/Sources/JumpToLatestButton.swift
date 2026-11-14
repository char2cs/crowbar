import SwiftUI

@available(iOS 26.0, *)
struct JumpToLatestButton: View {
  @ObservedObject var model: ChromeModel

  var body: some View {
    GlassCircle(
      icon: "ArrowDown",
      label: model.jumpLabel,
      tint: model.palette?.text
    ) {
      model.send(.of("jumpToLatest", ["owner": model.jumpOwner]))
    }
  }
}
