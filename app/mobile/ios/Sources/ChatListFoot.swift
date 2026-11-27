import SwiftUI

@available(iOS 26.0, *)
struct ChatListFoot: View {
  @ObservedObject var model: ChromeModel
  @Environment(\.colorScheme) private var colorScheme

  private var sidebar: SidebarPalette? { model.palette?.sidebar }
  private var ink: Color { sidebar?.text ?? .primary }

  var body: some View {
    GlassEffectContainer(spacing: Spacing.medium) { row }
      .padding(.horizontal, Spacing.large)
      .padding(.bottom, Spacing.small)
      .dynamicTypeSize(...DynamicTypeSize.xxxLarge)
      .environment(\.colorScheme, sidebar == nil ? colorScheme : .dark)
  }

  private var row: some View {
    HStack(spacing: Spacing.medium) {
      settings
      hosts
      Spacer(minLength: 0)
      newThread
    }
  }

  private var settings: some View {
    Button(
      action: { open("openSettings") },
      label: {
        PhosphorIcon(.gear)
          .frame(width: Metric.tapTarget, height: Metric.tapTarget)
          .foregroundStyle(ink)
          .contentShape(.circle)
      }
    )
    .buttonStyle(.plain)
    .glassEffect(.regular.interactive(), in: .circle)
    .accessibilityLabel("Settings")
  }

  private var hosts: some View {
    Button(
      action: { open("openHostPicker") },
      label: {
        HostPlaceIcon(
          hostId: model.hostPlace == "local" ? "local" : "remote",
          state: model.hostStatus,
          palette: model.palette
        )
        .frame(width: Metric.tapTarget, height: Metric.tapTarget)
        .foregroundStyle(ink)
        .contentShape(.circle)
      }
    )
    .buttonStyle(.plain)
    .glassEffect(.regular.interactive(), in: .circle)
    .accessibilityLabel("Hosts")
  }

  private var newThread: some View {
    Button(action: { open("newThread") }, label: { newThreadLabel })
      .buttonStyle(.glassProminent)
      .tint(sidebar?.brand ?? model.brandGround ?? .green)
      .controlSize(.large)
  }

  private var newThreadLabel: some View {
    HStack(spacing: Spacing.snug) {
      PhosphorIcon(.plus, size: IconSize.compact)
      Text("New thread")
    }
    .font(.subheadline.weight(.semibold))
    .padding(.horizontal, Spacing.snug)
    .foregroundStyle(sidebar?.brandText ?? .white)
  }

  private var open: (String) -> Void {
    { intent in
      Haptics.tap()
      model.send(.of(intent))
    }
  }
}
