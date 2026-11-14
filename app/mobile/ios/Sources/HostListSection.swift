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
      Button {
        Haptics.select()
        send(.of("addHost"))
      } label: {
        IconRow(icon: .plus, title: "Add host", text: text)
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
      Button {
        Haptics.select()
        send(.of("selectHost", ["id": host.id]))
      } label: {
        HostLine(host: host, text: text, secondaryText: secondaryText, palette: palette) {
          if revoking {
            ProgressView()
          } else if shown {
            PhosphorIcon(.check, size: 18)
          }
        }
        .contentShape(.rect)
      }
      .tint(text)
      .disabled(revoking)
      // PLATFORM: Not `role: .destructive`: that has the list drop the row at once, while the host stays in
      // PLATFORM: the model until the web has asked and revoked, and the next update then miscounts.
      .swipeActions(edge: .trailing, allowsFullSwipe: false) {
        Button {
          send(.of("revokeHost", ["id": host.id]))
        } label: {
          Label {
            Text("Revoke")
          } icon: {
            Image(uiImage: CrowbarIcon.trash.image(size: 22))
          }
        }
        .tint(.red)
      }
      .accessibilityAddTraits(shown ? .isSelected : [])
    }
  }
}
