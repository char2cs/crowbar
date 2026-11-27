import Foundation

struct ConnectState: Decodable, Equatable {
  let open: Bool
  let stage: ConnectStage
  let name: String
  let address: String
  let code: String
  let fingerprint: String
  let busy: Bool
  let error: String?
}
