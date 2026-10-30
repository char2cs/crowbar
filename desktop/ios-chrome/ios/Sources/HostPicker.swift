import SwiftUI

struct PickerHost: Decodable, Identifiable, Equatable {
  let id: String
  let name: String
  let endpoint: String
  /// The web's HostDotState.
  let state: String
}

/// Every paired host, as the web's hosts snapshot hands it over: the picker's and Settings' alike.
struct HostListState: Decodable, Equatable {
  let hosts: [PickerHost]
  let revoking: [String]
  /// The host whose projects the chat list shows.
  let shownHost: String?

  static let empty = HostListState(hosts: [], revoking: [], shownHost: nil)
}

/// The host picker, as `set_host_picker` hands it over.
struct HostPickerState: Decodable, Equatable {
  let open: Bool
  let list: HostListState

  private enum Keys: String, CodingKey { case open }

  init(open: Bool, list: HostListState) {
    self.open = open
    self.list = list
  }

  init(from decoder: Decoder) throws {
    open = try decoder.container(keyedBy: Keys.self).decode(Bool.self, forKey: .open)
    list = try HostListState(from: decoder)
  }
}

final class HostPickerModel: ObservableObject {
  @Published var state = HostPickerState(open: false, list: .empty)
  var send: (ChromeIntent) -> Void = { _ in }
}

/// The chat list's host picker in the model picker's clothes: tap a host to show it, swipe to
/// revoke it, Add host to pair another.
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
            Button { model.send(.of("closeHostPicker")) } label: { PhosphorIcon(.x) }
              .accessibilityLabel("Close")
          }
        }
    }
  }
}

/// A list section of the paired hosts and Add host: tap shows a host, swipe revokes it. Its rows
/// keep the list's own card background: a custom row background slides away with the row when
/// it is swiped, leaving the revealed action on a broken card.
@available(iOS 26.0, *)
struct HostListSection: View {
  let list: HostListState
  let palette: ChromePalette?
  let send: (ChromeIntent) -> Void

  private var text: Color { palette?.text ?? .primary }
  private var secondaryText: Color { palette?.secondaryText ?? .secondary }

  var body: some View {
    Section {
      ForEach(list.hosts) { host in row(host) }
      Button {
        Haptics.select()
        send(.of("addHost"))
      } label: {
        IconRow(icon: .plus, title: "Add host", text: text)
      }
    }
  }

  private func row(_ host: PickerHost) -> some View {
    let shown = list.shownHost == host.id
    let revoking = list.revoking.contains(host.id)
    return Button {
      Haptics.select()
      send(.of("selectHost", ["id": host.id]))
    } label: {
      HostLine(host: host, text: text, secondaryText: secondaryText) {
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
    // Not `role: .destructive`: that has the list drop the row at once, while the host stays in
    // the model until the web has asked and revoked, and the next update then miscounts.
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

/// The one row layout of every host and settings row: a 24pt icon column, a 12pt gap, the text,
/// and a 20pt trailing column, so rows on different screens line up.
enum RowMetrics {
  static let icon: CGFloat = 24
  static let gap: CGFloat = 12
  static let trailing: CGFloat = 20
}

/// A Phosphor icon and a title in the shared row layout.
struct IconRow: View {
  let icon: CrowbarIcon?
  let title: String
  let text: Color
  var value: String?
  var secondaryText: Color = .secondary

  var body: some View {
    HStack(spacing: RowMetrics.gap) {
      Group { if let icon { PhosphorIcon(icon) } }.frame(width: RowMetrics.icon)
      Text(title).lineLimit(1)
      if let value {
        Spacer(minLength: 8)
        Text(value).foregroundStyle(secondaryText).lineLimit(1)
      }
    }
    .foregroundStyle(text)
  }
}

/// A host's place icon with its status dot, its name and endpoint, then whatever trails, in the
/// shared row layout.
struct HostLine<Trailing: View>: View {
  let host: PickerHost
  let text: Color
  let secondaryText: Color
  @ViewBuilder let trailing: () -> Trailing

  var body: some View {
    HStack(spacing: RowMetrics.gap) {
      HostPlaceIcon(hostId: host.id, state: host.state).frame(width: RowMetrics.icon)
      VStack(alignment: .leading, spacing: 2) {
        Text(host.name).foregroundStyle(text).lineLimit(1)
        Text(host.endpoint).crowbarUI(.footnote).foregroundStyle(secondaryText).lineLimit(1)
      }
      Spacer(minLength: 8)
      trailing().frame(width: RowMetrics.trailing)
    }
  }
}

/// Where a host runs, as the web's `HostStatusIcon` draws it: this computer or a paired
/// remote's cloud, with the status dot on its bottom-right corner and the icon cut away around it.
struct HostPlaceIcon: View {
  let hostId: String
  let state: String

  var body: some View {
    PhosphorIcon(hostId == "local" ? .desktop : .cloud, size: 20)
      .mask {
        Rectangle().overlay(alignment: .bottomTrailing) {
          Circle().frame(width: 10, height: 10).offset(x: 1, y: 1).blendMode(.destinationOut)
        }
        .compositingGroup()
      }
      .overlay(alignment: .bottomTrailing) { dot.offset(x: 1, y: 1) }
      .accessibilityLabel(HostDot.words(state))
  }

  @ViewBuilder private var dot: some View {
    switch HostDot.tone(state) {
    case .unsettled:
      Circle().strokeBorder(Color.secondary, lineWidth: 1).frame(width: 7, height: 7)
    case let tone:
      Circle().fill(HostDot.color(tone)).frame(width: 7, height: 7)
    }
  }
}

/// A host state's tone, as the web's `hostTone` maps it, and the words it says it in.
enum HostDot {
  enum Tone { case online, offline, needsAttention, unsettled }

  static func tone(_ state: String) -> Tone {
    switch state {
    case "up", "no-live-stream": .online
    case "down", "not-available": .offline
    case "needs-repair": .needsAttention
    default: .unsettled
    }
  }

  static func color(_ tone: Tone) -> Color {
    switch tone {
    case .online: .green
    case .offline: .red
    case .needsAttention: .orange
    case .unsettled: .gray
    }
  }

  static func words(_ state: String) -> String {
    switch state {
    case "up": "Online"
    case "down": "Offline"
    case "needs-repair": "Needs pairing again"
    case "no-live-stream": "Online, no live stream"
    case "not-available": "Host not available"
    case "probing": "Checking"
    default: "Status unknown"
    }
  }
}
