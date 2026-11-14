import SwiftUI

@available(iOS 26.0, *)
struct ConnectFields: View {
  let stage: ConnectStage
  let fingerprint: String
  let palette: ChromePalette?
  @Binding var drafts: ConnectDrafts
  @FocusState private var focused: Bool

  private var text: Color { palette?.text ?? .primary }
  private var secondaryText: Color { palette?.secondaryText ?? .secondary }
  private var raised: Color { palette?.raised ?? Color(uiColor: .tertiarySystemFill) }

  var body: some View {
    Group {
      switch stage {
      case .address: address
      case .fingerprint: fingerprintCheck
      case .code: code
      }
    }
    .foregroundStyle(text)
  }

  private var address: some View {
    VStack(alignment: .leading, spacing: 14) {
      Text("Run crowbar pair on the host first: it shows the fingerprint and the code you’ll need.")
        .font(.subheadline)
        .foregroundStyle(secondaryText)
      Field(label: "Name", raised: raised) {
        TextField("homelab", text: $drafts.name)
          .accessibilityLabel("Name")
          .textInputAutocapitalization(.never)
          .autocorrectionDisabled()
      }
      Field(label: "Address", raised: raised) {
        TextField("homelab.example.com or 1.2.3.4", text: $drafts.address)
          .accessibilityLabel("Address")
          .textInputAutocapitalization(.never)
          .autocorrectionDisabled()
          .keyboardType(.URL)
          .font(.body.monospaced())
      }
      Text("Add a port, like :4000, if the host is not on the default one.")
        .font(.footnote)
        .foregroundStyle(secondaryText)
    }
  }

  private var fingerprintCheck: some View {
    VStack(alignment: .leading, spacing: 12) {
      Text("Confirm this matches the fingerprint printed by that host’s CLI:")
        .font(.subheadline)
        .foregroundStyle(secondaryText)
      Text(fingerprint)
        .font(.footnote.monospaced())
        .textSelection(.enabled)
        .frame(maxWidth: .infinity, alignment: .leading)
        .padding(14)
        .background(raised, in: .rect(cornerRadius: 16))
    }
  }

  private var code: some View {
    Field(label: "Pairing code", raised: raised) {
      TextField("123456", text: $drafts.code)
        .accessibilityLabel("Pairing code")
        .keyboardType(.numberPad)
        .textContentType(.oneTimeCode)
        .font(.title3.monospaced())
        .focused($focused)
        .onAppear { focused = true }
    }
  }

  private struct Field<Input: View>: View {
    let label: String
    let raised: Color
    @ViewBuilder let input: () -> Input

    var body: some View {
      VStack(alignment: .leading, spacing: 6) {
        Text(label).font(.subheadline.weight(.medium))
        input()
          .padding(.horizontal, 14)
          .frame(minHeight: 48)
          .background(raised, in: .rect(cornerRadius: 14))
      }
    }
  }
}
