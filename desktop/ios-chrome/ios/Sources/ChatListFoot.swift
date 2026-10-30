import SwiftUI

/// The chat list's own buttons, native over the bottom of the list page.
@available(iOS 26.0, *)
struct ChatListFoot: View {
  @ObservedObject var model: ChromeModel
  @Environment(\.colorScheme) private var colorScheme

  /// Over the dark chrome sidebar (Crowbar) the foot reads as dark glass with light glyphs and a
  /// brand-green New thread; nil keeps the plain look.
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
          PhosphorIcon(model.hostPlace == "local" ? .desktop : .cloud)
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
        // Over the dark sidebar the brand green is the dark palette's primary; else the welcome's.
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
