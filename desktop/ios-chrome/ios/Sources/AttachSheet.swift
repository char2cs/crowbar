import SwiftUI

struct AttachSource: Decodable, Identifiable, Equatable {
  let id: String
  let title: String
  /// A Phosphor name.
  let icon: String
}

struct AttachFile: Decodable, Identifiable, Equatable {
  let id: String
  let name: String
  let detail: String
  /// `uploading`, `done` or `bad`.
  let status: String
  /// 0-100 once bytes are reported; nil draws an indeterminate bar.
  let percent: Int?
  let canRetry: Bool
}

/// The composer's Attach sheet, as `set_attach_sheet` hands it over; the web owns every upload.
struct AttachSheetState: Decodable, Equatable {
  let open: Bool
  let title: String
  let note: String
  let sources: [AttachSource]
  let files: [AttachFile]
  let hint: String?
  let attachLabel: String
  let canAttach: Bool
}

final class AttachSheetModel: ObservableObject {
  @Published var state = AttachSheetState(
    open: false, title: "", note: "", sources: [], files: [], hint: nil, attachLabel: "",
    canAttach: false)
}

/// The Attach sheet as a native sheet in the web palette. It only draws: every tap is reported as
/// an intent and the web answers with the next state.
@available(iOS 26.0, *)
struct AttachSheet: View {
  @ObservedObject var model: AttachSheetModel
  @ObservedObject var chrome: ChromeModel

  private var state: AttachSheetState { model.state }
  private var palette: ChromePalette? { chrome.palette }
  private var text: Color { palette?.text ?? .primary }
  private var secondaryText: Color { palette?.secondaryText ?? .secondary }

  var body: some View {
    NavigationStack {
      List {
        Section {
          sources
          Text(state.note)
            .font(.footnote)
            .foregroundStyle(secondaryText)
            .listRowBackground(Color.clear)
            .listRowInsets(EdgeInsets(top: 4, leading: 8, bottom: 0, trailing: 8))
        }
        if !state.files.isEmpty {
          Section {
            ForEach(state.files) { file in row(file) }
          }
          .listRowBackground(palette?.raised)
        }
      }
      .nativeList(palette)
      .nativeTitle(state.title)
      .toolbarBackgroundVisibility(.hidden, for: .navigationBar)
      .safeAreaInset(edge: .bottom) { actions }
    }
  }

  private var sources: some View {
    GlassEffectContainer(spacing: 8) {
      HStack(spacing: 8) {
        ForEach(state.sources) { source in
          Button {
            Haptics.select()
            chrome.send(.of("pickAttachSource", ["id": source.id]))
          } label: {
            HStack(spacing: 6) {
              if let icon = CrowbarIcon(named: source.icon) { PhosphorIcon(icon, size: 18) }
              Text(source.title).lineLimit(1)
            }
            .frame(maxWidth: .infinity, minHeight: 44)
            .background(palette?.raised ?? Color.secondary.opacity(0.15), in: .capsule)
            .contentShape(.capsule)
          }
          .buttonStyle(.plain)
          .foregroundStyle(text)
        }
      }
    }
    .listRowBackground(Color.clear)
    .listRowInsets(EdgeInsets())
  }

  private func row(_ file: AttachFile) -> some View {
    HStack(alignment: .top, spacing: RowMetrics.gap) {
      VStack(alignment: .leading, spacing: 4) {
        Text(file.name).lineLimit(1).foregroundStyle(text)
        HStack(spacing: 8) {
          Text(file.detail)
            .font(.footnote)
            .foregroundStyle(file.status == "bad" ? Color.red : secondaryText)
          if file.canRetry {
            Button("Retry") {
              Haptics.select()
              chrome.send(.of("retryAttachFile", ["id": file.id]))
            }
            .font(.footnote.weight(.semibold))
            .buttonStyle(.glass)
            .buttonBorderShape(.capsule)
            .controlSize(.small)
          }
        }
        if file.status == "uploading" { progress(file.percent) }
      }
      .frame(maxWidth: .infinity, alignment: .leading)
      Button {
        Haptics.select()
        chrome.send(.of("removeAttachFile", ["id": file.id]))
      } label: {
        PhosphorIcon(.x, size: 16, emphasis: 0.5)
          .foregroundStyle(secondaryText)
          .frame(width: 44, height: 44)
          .contentShape(.rect)
      }
      .buttonStyle(.plain)
      .accessibilityLabel("Remove \(file.name)")
    }
    .listRowInsets(EdgeInsets(top: 4, leading: 16, bottom: 4, trailing: 4))
  }

  @ViewBuilder
  private func progress(_ percent: Int?) -> some View {
    if let percent {
      ProgressView(value: Double(percent), total: 100).tint(chrome.accent)
    } else {
      ProgressView().progressViewStyle(.linear).tint(chrome.accent)
    }
  }

  private var actions: some View {
    VStack(spacing: 8) {
      if let hint = state.hint {
        Text(hint).font(.footnote).foregroundStyle(secondaryText)
      }
      GlassEffectContainer(spacing: 12) {
        HStack(spacing: 12) {
          Button { chrome.send(.of("closeAttachSheet")) } label: {
            Text("Cancel")
              .frame(maxWidth: .infinity, minHeight: 48)
              .background(palette?.raised ?? Color.secondary.opacity(0.15), in: .capsule)
              .contentShape(.capsule)
          }
          .buttonStyle(.plain)
          .foregroundStyle(text)

          Button {
            Haptics.select()
            chrome.send(.of("attachFiles"))
          } label: {
            Text(state.attachLabel).frame(maxWidth: .infinity)
          }
          .buttonStyle(.glassProminent)
          .tint(chrome.accent)
          .disabled(!state.canAttach)
        }
        .buttonBorderShape(.capsule)
        .controlSize(.large)
        .font(.headline)
      }
    }
    .padding(.horizontal, 16)
    .padding(.top, 8)
    .padding(.bottom, 12)
    .background(palette?.surface.opacity(0.9))
  }
}
