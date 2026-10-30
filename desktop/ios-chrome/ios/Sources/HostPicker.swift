import SwiftUI

struct PickerHost: Decodable, Identifiable, Equatable {
  let id: String
  let name: String
  let endpoint: String
  /// The web's HostDotState.
  let state: String
}

/// The host picker, as `set_host_picker` hands it over.
struct HostPickerState: Decodable, Equatable {
  let open: Bool
  let hosts: [PickerHost]
  let revoking: [String]
  /// The host whose projects the chat list shows.
  let shownHost: String?
}

final class HostPickerModel: ObservableObject {
  @Published var state = HostPickerState(open: false, hosts: [], revoking: [], shownHost: nil)
  var send: (ChromeIntent) -> Void = { _ in }
}

/// The chat list's host picker in the model picker's clothes: tap a host to show it, swipe to
/// revoke it, Add host to pair another.
@available(iOS 26.0, *)
struct HostPickerSheet: View {
  @ObservedObject var model: HostPickerModel
  @ObservedObject var chrome: ChromeModel

  private var palette: ChromePalette? { chrome.palette }
  private var text: Color { palette?.text ?? .primary }
  private var secondaryText: Color { palette?.secondaryText ?? .secondary }

  var body: some View {
    NavigationStack {
      List {
        Section {
          ForEach(model.state.hosts) { host in row(host) }
          Button {
            Haptics.select()
            model.send(.of("addHost"))
          } label: {
            Label("Add host", systemImage: "plus").foregroundStyle(text)
          }
        }
        .listRowBackground(palette?.raised)
      }
      .listStyle(.insetGrouped)
      .scrollContentBackground(palette == nil ? .automatic : .hidden)
      .background(palette?.surface.ignoresSafeArea())
      .navigationTitle("Hosts")
      .navigationBarTitleDisplayMode(.inline)
      .toolbarBackgroundVisibility(.hidden, for: .navigationBar)
      .toolbar {
        ToolbarItem(placement: .topBarLeading) {
          Button(role: .close) { model.send(.of("closeHostPicker")) }
        }
      }
    }
  }

  private func row(_ host: PickerHost) -> some View {
    let shown = model.state.shownHost == host.id
    let revoking = model.state.revoking.contains(host.id)
    return Button {
      Haptics.select()
      model.send(.of("selectHost", ["id": host.id]))
    } label: {
      HStack(spacing: 12) {
        Circle().fill(HostDot.color(host.state)).frame(width: 10, height: 10)
        VStack(alignment: .leading, spacing: 2) {
          Text(host.name).foregroundStyle(text)
          Text(host.endpoint).font(.footnote).foregroundStyle(secondaryText)
        }
        Spacer()
        if revoking {
          ProgressView()
        } else if shown {
          Image(systemName: "checkmark").fontWeight(.semibold)
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
        model.send(.of("revokeHost", ["id": host.id]))
      } label: {
        Label("Revoke", systemImage: "trash")
      }
      .tint(.red)
    }
    .accessibilityAddTraits(shown ? .isSelected : [])
  }
}

/// A host status dot's colour: up is green, a host still being asked is neutral, one that
/// answers without a live stream is amber, and anything else needs attention.
enum HostDot {
  static func color(_ state: String) -> Color {
    switch state {
    case "up": .green
    case "unknown", "probing": .gray
    case "no-live-stream": .orange
    default: .red
    }
  }
}
