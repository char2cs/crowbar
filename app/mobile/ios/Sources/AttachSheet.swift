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
    NavigationStack { list }
  }

  private var list: some View {
    List {
      sourcesSection
      filesSection
    }
    .nativeList(palette)
    .nativeTitle(state.title)
    .toolbarBackgroundVisibility(.hidden, for: .navigationBar)
    .safeAreaInset(edge: .bottom) { actions }
  }

  private var sourcesSection: some View {
    Section {
      sources
      note
    }
  }

  private var note: some View {
    Text(state.note)
      .font(.footnote)
      .foregroundStyle(secondaryText)
      .listRowBackground(Color.clear)
      .listRowInsets(noteInsets)
  }

  private var noteInsets: EdgeInsets {
    EdgeInsets(
      top: Spacing.tight,
      leading: Spacing.small,
      bottom: 0,
      trailing: Spacing.small
    )
  }

  @ViewBuilder private var filesSection: some View {
    if !state.files.isEmpty {
      Section { fileRows }
        .listRowBackground(palette?.raised)
    }
  }

  private var fileRows: some View {
    ForEach(state.files) { file in
      Row(
        file: file,
        text: text,
        secondaryText: secondaryText,
        accent: accent,
        send: chrome.send
      )
    }
  }

  private var sources: some View {
    GlassEffectContainer(spacing: Spacing.small) {
      HStack(spacing: Spacing.small) { sourceButtons }
    }
    .listRowBackground(Color.clear)
    .listRowSeparator(.hidden)
    .listRowInsets(EdgeInsets())
  }

  private var sourceButtons: some View {
    ForEach(state.sources) { source in
      SourceButton(
        source: source,
        palette: palette,
        text: text,
        pick: pick
      )
    }
  }

  private struct SourceButton: View {
    let source: AttachSource
    let palette: ChromePalette?
    let text: Color
    let pick: (String) -> Void

    var body: some View {
      Button(action: choose) { label }
        .buttonStyle(.plain)
        .foregroundStyle(text)
    }

    private var choose: () -> Void {
      {
        Haptics.select()
        pick(source.id)
      }
    }

    private var label: some View {
      HStack(spacing: Spacing.snug) {
        icon
        Text(source.title).lineLimit(1)
      }
      .frame(maxWidth: .infinity, minHeight: Metric.tapTarget)
      .background(palette?.raised ?? Color.secondary.opacity(Alpha.veil), in: .capsule)
      .contentShape(.capsule)
    }

    @ViewBuilder private var icon: some View {
      if let icon = CrowbarIcon(named: source.icon) {
        PhosphorIcon(icon, size: IconSize.standard)
      }
    }
  }

  private struct Row: View {
    let file: AttachFile
    let text: Color
    let secondaryText: Color
    let accent: Color?
    let send: (ChromeIntent) -> Void

    var body: some View {
      HStack(alignment: .top, spacing: RowMetrics.gap) {
        details
        remove
      }
      .listRowInsets(insets)
    }

    private var insets: EdgeInsets {
      EdgeInsets(
        top: Spacing.tight,
        leading: Spacing.large,
        bottom: Spacing.tight,
        trailing: Spacing.tight
      )
    }

    private var details: some View {
      VStack(alignment: .leading, spacing: Spacing.tight) {
        Text(file.name).lineLimit(1).foregroundStyle(text)
        status
        if file.status == "uploading" { progress }
      }
      .frame(maxWidth: .infinity, alignment: .leading)
    }

    private var status: some View {
      HStack(spacing: Spacing.small) {
        Text(file.detail)
          .font(.footnote)
          .foregroundStyle(file.status == "bad" ? Color.red : secondaryText)
        retry
      }
    }

    @ViewBuilder private var retry: some View {
      if file.canRetry {
        Button("Retry", action: retryFile)
          .font(.footnote.weight(.semibold))
          .buttonStyle(.glass)
          .buttonBorderShape(.capsule)
          .controlSize(.small)
      }
    }

    private var retryFile: () -> Void {
      {
        Haptics.select()
        send(.of("retryAttachFile", ["id": file.id]))
      }
    }

    private var removeFile: () -> Void {
      {
        Haptics.select()
        send(.of("removeAttachFile", ["id": file.id]))
      }
    }

    private var remove: some View {
      Button(action: removeFile) {
        PhosphorIcon(.xMark, size: IconSize.standard, emphasis: Metric.strongEmphasis)
          .foregroundStyle(secondaryText)
          .frame(width: Metric.tapTarget, height: Metric.tapTarget)
          .contentShape(.rect)
      }
      .buttonStyle(.plain)
      .accessibilityLabel("Remove \(file.name)")
    }

    private var progress: some View {
      ProgressView(value: file.percent.map(Double.init), total: Metric.percentTotal)
        .progressViewStyle(.linear)
        .tint(accent)
    }
  }

  private var actions: some View {
    VStack(spacing: Spacing.small) {
      hint
      buttons
    }
    .padding(.horizontal, Spacing.large)
    .padding(.top, Spacing.small)
    .padding(.bottom, Spacing.medium)
    .background(palette?.surface.opacity(Alpha.scrim))
  }

  @ViewBuilder private var hint: some View {
    if let hint = state.hint {
      Text(hint).font(.footnote).foregroundStyle(secondaryText)
    }
  }

  private var buttons: some View {
    GlassEffectContainer(spacing: Spacing.medium) { buttonRow }
  }

  private var buttonRow: some View {
    HStack(spacing: Spacing.medium) {
      cancel
      attach
    }
    .buttonBorderShape(.capsule)
    .controlSize(.large)
    .font(.headline)
  }

  private var cancel: some View {
    Button(
      action: { chrome.send(.of("closeAttachSheet")) },
      label: { cancelLabel }
    )
    .buttonStyle(.plain)
    .foregroundStyle(text)
  }

  private var cancelLabel: some View {
    Text("Cancel")
      .frame(maxWidth: .infinity, minHeight: Metric.controlHeight)
      .background(palette?.raised ?? Color.secondary.opacity(Alpha.veil), in: .capsule)
      .contentShape(.capsule)
  }

  private var attachLabel: some View {
    Text(state.attachLabel).frame(maxWidth: .infinity)
  }

  private var attach: some View {
    Button(action: attachFiles) { attachLabel }
      .buttonStyle(.glassProminent)
      .tint(accent)
      .disabled(!state.canAttach)
  }

  private var attachFiles: () -> Void {
    {
      Haptics.select()
      chrome.send(.of("attachFiles"))
    }
  }
}
