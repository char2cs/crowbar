import SwiftUI

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
        .padding(.horizontal, Spacing.snug)
    }
    .buttonStyle(.glassProminent)
    .tint(.white)
    .foregroundStyle(.black)
    .controlSize(.large)
    .fixedSize()
    .dynamicTypeSize(...DynamicTypeSize.xxxLarge)
  }
}
