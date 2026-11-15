import Foundation

struct SchemeReply {
  let url: URL
  let status: Int
  var headers: [String: String] = [:]
  var body = Data()
}
