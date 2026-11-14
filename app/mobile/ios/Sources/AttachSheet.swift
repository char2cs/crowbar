import SwiftUI

@available(iOS 26.0, *)
struct AttachSheet: View {
  @ObservedObject var model: AttachSheetModel
  @ObservedObject var chrome: ChromeModel
  let pick: (String) -> Void

  @Environment(\.colorScheme) private var scheme

  private var state: AttachSheetState { model.state }
  private var accent: Color? { palette?.brand(scheme) ?? chrome.accent }
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
            ForEach(state.files) { file in
              Row(
                file: file,
                text: text,
                secondaryText: secondaryText,
                accent: accent,
                send: { chrome.send($0) }
              )
            }
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
            pick(source.id)
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
    .listRowSeparator(.hidden)
    .listRowInsets(EdgeInsets())
  }

  private struct Row: View {
    let file: AttachFile
    let text: Color
    let secondaryText: Color
    let accent: Color?
    let send: (ChromeIntent) -> Void

    var body: some View {
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
                send(.of("retryAttachFile", ["id": file.id]))
              }
              .font(.footnote.weight(.semibold))
              .buttonStyle(.glass)
              .buttonBorderShape(.capsule)
              .controlSize(.small)
            }
          }
          if file.status == "uploading" { progress }
        }
        .frame(maxWidth: .infinity, alignment: .leading)
        Button {
          Haptics.select()
          send(.of("removeAttachFile", ["id": file.id]))
        } label: {
          PhosphorIcon(.x, size: 18, emphasis: 0.7)
            .foregroundStyle(secondaryText)
            .frame(width: 44, height: 44)
            .contentShape(.rect)
        }
        .buttonStyle(.plain)
        .accessibilityLabel("Remove \(file.name)")
      }
      .listRowInsets(EdgeInsets(top: 4, leading: 16, bottom: 4, trailing: 4))
    }

    @ViewBuilder private var progress: some View {
      if let percent = file.percent {
        ProgressView(value: Double(percent), total: 100).tint(accent)
      } else {
        ProgressView().progressViewStyle(.linear).tint(accent)
      }
    }
  }

  private var actions: some View {
    VStack(spacing: 8) {
      if let hint = state.hint {
        Text(hint).font(.footnote).foregroundStyle(secondaryText)
      }
      GlassEffectContainer(spacing: 12) {
        HStack(spacing: 12) {
          Button {
            chrome.send(.of("closeAttachSheet"))
          } label: {
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
          .tint(accent)
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
