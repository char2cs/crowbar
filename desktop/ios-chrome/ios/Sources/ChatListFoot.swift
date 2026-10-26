import SwiftUI

/// The chat list's own buttons, native over the bottom of the list page.
@available(iOS 26.0, *)
struct ChatListFoot: View {
  @ObservedObject var model: ChromeModel

  var body: some View {
    GlassEffectContainer(spacing: 12) {
      HStack(spacing: 12) {
        Button {
          Haptics.tap()
          model.send(.of("openSettings"))
        } label: {
          Image(systemName: "gearshape")
            .font(.body.weight(.medium))
            .frame(width: 44, height: 44)
            .contentShape(.circle)
        }
        .buttonStyle(.plain)
        .glassEffect(.regular.interactive(), in: .circle)
        .accessibilityLabel("Settings")

        Spacer(minLength: 0)

        Button {
          Haptics.tap()
          model.send(.of("newThread"))
        } label: {
          Label("New thread", systemImage: "plus")
            .font(.subheadline.weight(.semibold))
            .padding(.horizontal, 6)
            .foregroundStyle(.white)
        }
        .buttonStyle(.glassProminent)
        // The welcome screen's green for the current theme.
        .tint(model.brandGround ?? .green)
        .controlSize(.large)
      }
    }
    .padding(.horizontal, 16)
    .padding(.bottom, 8)
    .dynamicTypeSize(...DynamicTypeSize.xxxLarge)
  }
}
