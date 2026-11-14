import SwiftUI

@available(iOS 26.0, *)
struct ChatListFoot: View {
  @ObservedObject var model: ChromeModel
  @Environment(\.colorScheme) private var colorScheme

  private var sidebar: SidebarPalette? { model.palette?.sidebar }

  var body: some View {
    GlassEffectContainer(spacing: 12) {
      HStack(spacing: 12) {
        Button {
          Haptics.tap()
          model.send(.of("openSettings"))
        } label: {
          PhosphorIcon(.gear)
            .frame(width: 44, height: 44)
            .foregroundStyle(sidebar?.text ?? .primary)
            .contentShape(.circle)
        }
        .buttonStyle(.plain)
        .glassEffect(.regular.interactive(), in: .circle)
        .accessibilityLabel("Settings")

        Button {
          Haptics.tap()
          model.send(.of("openHostPicker"))
        } label: {
          HostPlaceIcon(
            hostId: model.hostPlace == "local" ? "local" : "remote",
            state: model.hostStatus,
            palette: model.palette
          )
          .frame(width: 44, height: 44)
          .foregroundStyle(sidebar?.text ?? .primary)
          .contentShape(.circle)
        }
        .buttonStyle(.plain)
        .glassEffect(.regular.interactive(), in: .circle)
        .accessibilityLabel("Hosts")

        Spacer(minLength: 0)

        Button {
          Haptics.tap()
          model.send(.of("newThread"))
        } label: {
          HStack(spacing: 6) {
            PhosphorIcon(.plus, size: 16)
            Text("New thread")
          }
          .font(.subheadline.weight(.semibold))
          .padding(.horizontal, 6)
          .foregroundStyle(sidebar?.brandText ?? .white)
        }
        .buttonStyle(.glassProminent)
        .tint(sidebar?.brand ?? model.brandGround ?? .green)
        .controlSize(.large)
      }
    }
    .padding(.horizontal, 16)
    .padding(.bottom, 8)
    .dynamicTypeSize(...DynamicTypeSize.xxxLarge)
    .environment(\.colorScheme, sidebar == nil ? colorScheme : .dark)
  }
}
