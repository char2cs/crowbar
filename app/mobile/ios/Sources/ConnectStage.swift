import Foundation

enum ConnectStage: String, Decodable {
  case address, fingerprint, code
}
