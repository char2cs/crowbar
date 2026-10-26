import SwiftUI

/// The first-run call to action, centred under the web-drawn brand.
@available(iOS 26.0, *)
struct WelcomeButton: View {
  @ObservedObject var model: ChromeModel

  var body: some View {
    Button {
      Haptics.tap()
      model.send(.start)
    } label: {
      Text(model.welcome?.label ?? "")
        .font(.headline)
        .padding(.horizontal, 6)
    }
    .buttonStyle(.glassProminent)
    .tint(.white)
    .foregroundStyle(.black)
    .controlSize(.large)
    .fixedSize()
    .dynamicTypeSize(...DynamicTypeSize.xxxLarge)
  }
}
