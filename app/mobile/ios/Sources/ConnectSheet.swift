import SwiftUI

/// Pairing with a remote daemon, one stage at a time as the web hands it over. The fields are
/// drafts the user is typing; the web owns what was submitted and every other piece of state.
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
          stage: stage, fingerprint: state?.fingerprint ?? "", palette: palette, drafts: $drafts)
          .disabled(busy)
          .frame(maxWidth: .infinity, alignment: .leading)
      }
      .scrollBounceBehavior(.basedOnSize)
      // Beside the actions, never scrolled out of a half-height sheet.
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
        Button { model.send(stage == .address ? .dismiss : .back) } label: {
          Text(stage == .address ? "Cancel" : "Back").frame(maxWidth: .infinity)
        }
        .buttonStyle(.glass)
        .disabled(busy)

        Button(action: submit) {
          HStack(spacing: 8) {
            if busy { ProgressView() }
            Text(primaryLabel)
          }
          .frame(maxWidth: .infinity)
        }
        .buttonStyle(.glassProminent)
        .tint(model.accent)
        .disabled(busy || !canSubmit)
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

  private var canSubmit: Bool {
    switch stage {
    case .address: return !drafts.trimmedName.isEmpty && !drafts.trimmedAddress.isEmpty
    case .fingerprint: return true
    case .code: return !drafts.code.isEmpty
    }
  }

  private func submit() {
    switch stage {
    case .address: model.send(.submitAddress(name: drafts.name, address: drafts.address))
    case .fingerprint: model.send(.confirmFingerprint)
    case .code: model.send(.submitCode(drafts.code))
    }
  }
}

/// What is typed but not yet submitted, seeded from the web's state when the sheet appears.
struct ConnectDrafts {
  var name = ""
  var address = ""
  var code = ""

  init() {}

  init(from state: ConnectState?) {
    name = state?.name ?? ""
    address = state?.address ?? ""
    code = state?.code ?? ""
  }

  var trimmedName: String { name.trimmingCharacters(in: .whitespaces) }
  var trimmedAddress: String { address.trimmingCharacters(in: .whitespaces) }
}
