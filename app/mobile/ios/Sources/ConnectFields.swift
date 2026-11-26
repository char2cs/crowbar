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
    VStack(alignment: .leading, spacing: Spacing.roomy) {
      Text("On the host, open Settings → Devices and tap Add a device (or run crowbar pair on a headless host): it shows the address, fingerprint and code you’ll need.")
        .font(.subheadline)
        .foregroundStyle(secondaryText)
      nameField
      addressField
      Text("Add a port, like :4000, if the host is not on the default one.")
        .font(.footnote)
        .foregroundStyle(secondaryText)
    }
  }

  private var nameField: some View {
    Field(label: "Name", raised: raised) {
      TextField("homelab", text: $drafts.name)
        .accessibilityLabel("Name")
        .textInputAutocapitalization(.never)
        .autocorrectionDisabled()
    }
  }

  private var addressField: some View {
    Field(label: "Address", raised: raised) {
      TextField("homelab.example.com or 1.2.3.4", text: $drafts.address)
        .accessibilityLabel("Address")
        .textInputAutocapitalization(.never)
        .autocorrectionDisabled()
        .keyboardType(.URL)
        .font(.body.monospaced())
    }
  }

  private var fingerprintCheck: some View {
    VStack(alignment: .leading, spacing: Spacing.medium) {
      Text("Confirm this matches the fingerprint shown on that host:")
        .font(.subheadline)
        .foregroundStyle(secondaryText)
      Text(fingerprint)
        .font(.footnote.monospaced())
        .textSelection(.enabled)
        .frame(maxWidth: .infinity, alignment: .leading)
        .padding(Spacing.roomy)
        .background(raised, in: .rect(cornerRadius: Corner.card))
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
      VStack(alignment: .leading, spacing: Spacing.snug) {
        Text(label).font(.subheadline.weight(.medium))
        input()
          .padding(.horizontal, Spacing.roomy)
          .frame(minHeight: Metric.controlHeight)
          .background(raised, in: .rect(cornerRadius: Corner.row))
      }
    }
  }
}
