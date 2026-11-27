import SwiftUI

@available(iOS 26.0, *)
struct HostListSection: View {
  let list: HostListState
  let palette: ChromePalette?
  let send: (ChromeIntent) -> Void

  private var text: Color { palette?.text ?? .primary }
  private var secondaryText: Color { palette?.secondaryText ?? .secondary }

  var body: some View {
    Section {
      rows
      add
    }
  }

  private var rows: some View {
    ForEach(list.hosts) { host in
      Row(
        host: host,
        shown: list.shownHost == host.id,
        revoking: list.revoking.contains(host.id),
        text: text,
        secondaryText: secondaryText,
        palette: palette,
        send: send
      )
    }
  }

  private var add: some View {
    Button(action: addHost) { IconRow(icon: .plus, title: "Add host", text: text) }
  }

  private var addHost: () -> Void {
    {
      Haptics.select()
      send(.of("addHost"))
    }
  }

  private struct Trailing: View {
    let revoking: Bool
    let shown: Bool

    var body: some View {
      switch (revoking, shown) {
      case (true, _): ProgressView()
      case (false, true): PhosphorIcon(.check, size: IconSize.standard)
      case (false, false): EmptyView()
      }
    }
  }

  private struct RevokeLabel: View {
    var body: some View {
      Label {
        Text("Revoke")
      } icon: {
        Image(uiImage: CrowbarIcon.trash.image(size: IconSize.xlarge))
      }
    }
  }

  private struct Row: View {
    let host: PickerHost
    let shown: Bool
    let revoking: Bool
    let text: Color
    let secondaryText: Color
    let palette: ChromePalette?
    let send: (ChromeIntent) -> Void

    var body: some View {
      Button(action: select) { line }
        .tint(text)
        .disabled(revoking)
        // PLATFORM: ios: Not `role: .destructive`: that has the list drop the row at once,
        // PLATFORM: ios: while the host stays in the model until the web has revoked it.
        .swipeActions(edge: .trailing, allowsFullSwipe: false) { revoke }
        .accessibilityAddTraits(shown ? .isSelected : [])
    }

    private var select: () -> Void {
      {
        Haptics.select()
        send(.of("selectHost", ["id": host.id]))
      }
    }

    private var line: some View {
      HostLine(host: host, text: text, secondaryText: secondaryText, palette: palette) {
        Trailing(revoking: revoking, shown: shown)
      }
      .contentShape(.rect)
    }

    private var revoke: some View {
      Button(
        action: { send(.of("revokeHost", ["id": host.id])) },
        label: { RevokeLabel() }
      )
      .tint(.red)
    }
  }
}
