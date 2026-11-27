import SwiftUI

@available(iOS 26.0, *)
struct ConnectSheet: View {
  @ObservedObject var model: ChromeModel
  @State private var drafts = ConnectDrafts()

  private var state: ConnectState? { model.connect }
  private var busy: Bool { state?.busy ?? false }
  private var palette: ChromePalette? { model.palette }

  var body: some View {
    VStack(alignment: .leading, spacing: Spacing.wide) {
      header
      fields
      errorText
      actions
    }
    .padding(.horizontal, Spacing.xwide)
    .padding(.top, Spacing.hero)
    .padding(.bottom, Spacing.medium)
    .background(palette?.surface.ignoresSafeArea())
    .onAppear { drafts = ConnectDrafts(from: state) }
  }

  private var header: some View {
    VStack(alignment: .leading, spacing: Spacing.tight) {
      Text("Connect to a host").font(.title2.weight(.semibold))
        .foregroundStyle(palette?.text ?? .primary)
      Text("Pair this phone with a computer running Crowbar.")
        .font(.subheadline)
        .foregroundStyle(palette?.secondaryText ?? .secondary)
    }
  }

  private var fields: some View {
    ScrollView { connectFields }
      .scrollBounceBehavior(.basedOnSize)
  }

  private var connectFields: some View {
    ConnectFields(
      stage: stage,
      fingerprint: state?.fingerprint ?? "",
      palette: palette,
      drafts: $drafts
    )
    .disabled(busy)
    .frame(maxWidth: .infinity, alignment: .leading)
  }

  @ViewBuilder private var errorText: some View {
    if let error = state?.error {
      Text(error)
        .font(.footnote)
        .foregroundStyle(.red)
        .fixedSize(horizontal: false, vertical: true)
    }
  }

  private var stage: ConnectStage { state?.stage ?? .address }

  private var actions: some View {
    GlassEffectContainer(spacing: Spacing.medium) { buttons }
  }

  private var buttons: some View {
    HStack(spacing: Spacing.medium) {
      back
      primary
    }
    .controlSize(.large)
    .font(.headline)
  }

  private var back: some View {
    Button(
      action: { model.send(stage == .address ? .dismiss : .back) },
      label: { Text(stage == .address ? "Cancel" : "Back").frame(maxWidth: .infinity) }
    )
    .buttonStyle(.glass)
    .disabled(busy)
  }

  private var primary: some View {
    Button(
      action: { model.send(drafts.submission(at: stage)) },
      label: { primaryContent }
    )
    .buttonStyle(.glassProminent)
    .tint(model.accent)
    .disabled(busy || !drafts.canSubmit(at: stage))
  }

  private var primaryContent: some View {
    HStack(spacing: Spacing.small) {
      spinner
      Text(primaryLabel)
    }
    .frame(maxWidth: .infinity)
  }

  @ViewBuilder private var spinner: some View {
    if busy { ProgressView() }
  }

  private var primaryLabel: String {
    switch stage {
    case .address: return busy ? "Connecting…" : "Continue"
    case .fingerprint: return "Confirm"
    case .code: return busy ? "Pairing…" : "Pair"
    }
  }
}
