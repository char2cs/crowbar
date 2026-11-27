import Foundation

struct ConnectDrafts {
  var name = ""
  var address = ""
  var code = ""

  var trimmedName: String { name.trimmingCharacters(in: .whitespaces) }
  var trimmedAddress: String { address.trimmingCharacters(in: .whitespaces) }

  func canSubmit(at stage: ConnectStage) -> Bool {
    switch stage {
    case .address: return !trimmedName.isEmpty && !trimmedAddress.isEmpty
    case .fingerprint: return true
    case .code: return !code.isEmpty
    }
  }

  func submission(at stage: ConnectStage) -> ChromeIntent {
    switch stage {
    case .address: return .submitAddress(name: name, address: address)
    case .fingerprint: return .confirmFingerprint
    case .code: return .submitCode(code)
    }
  }
}

extension ConnectDrafts {
  init(from state: ConnectState?) {
    self.init(name: state?.name ?? "", address: state?.address ?? "", code: state?.code ?? "")
  }
}
