import SwiftUI

@available(iOS 26.0, *)
struct ConnectSheet: View {
  @ObservedObject var model: ChromeModel
  @State private var drafts = ConnectDrafts()

  private var state: ConnectState? { model.connect }
  private var busy: Bool { state?.busy ?? false }
  private var palette: ChromePalette? { model.palette }

  var body: some View {
    VStack(alignment: .leading, spacing: 20) {
      VStack(alignment: .leading, spacing: 4) {
        Text("Connect to a host").font(.title2.weight(.semibold))
          .foregroundStyle(palette?.text ?? .primary)
        Text("Pair this phone with a computer running Crowbar.")
          .font(.subheadline)
          .foregroundStyle(palette?.secondaryText ?? .secondary)
      }
      ScrollView {
        ConnectFields(
          stage: stage,
          fingerprint: state?.fingerprint ?? "",
          palette: palette,
          drafts: $drafts
        )
        .disabled(busy)
        .frame(maxWidth: .infinity, alignment: .leading)
      }
      .scrollBounceBehavior(.basedOnSize)
      if let error = state?.error {
        Text(error)
          .font(.footnote)
          .foregroundStyle(.red)
          .fixedSize(horizontal: false, vertical: true)
      }
      actions
    }
    .padding(.horizontal, 24)
    .padding(.top, 28)
    .padding(.bottom, 12)
    .background(palette?.surface.ignoresSafeArea())
    .onAppear { drafts = ConnectDrafts(from: state) }
  }

  private var stage: ConnectStage { state?.stage ?? .address }

  private var actions: some View {
    GlassEffectContainer(spacing: 12) {
      HStack(spacing: 12) {
        Button {
          model.send(stage == .address ? .dismiss : .back)
        } label: {
          Text(stage == .address ? "Cancel" : "Back").frame(maxWidth: .infinity)
        }
        .buttonStyle(.glass)
        .disabled(busy)

        Button {
          model.send(drafts.submission(at: stage))
        } label: {
          HStack(spacing: 8) {
            if busy { ProgressView() }
            Text(primaryLabel)
          }
          .frame(maxWidth: .infinity)
        }
        .buttonStyle(.glassProminent)
        .tint(model.accent)
        .disabled(busy || !drafts.canSubmit(at: stage))
      }
      .controlSize(.large)
      .font(.headline)
    }
  }

  private var primaryLabel: String {
    switch stage {
    case .address: return busy ? "Connecting…" : "Continue"
    case .fingerprint: return "Confirm"
    case .code: return busy ? "Pairing…" : "Pair"
    }
  }
}
