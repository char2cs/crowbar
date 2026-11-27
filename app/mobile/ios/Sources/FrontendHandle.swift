import Foundation

struct FrontendHandle: Decodable {
  let frontend: UInt

  var pointer: UnsafeRawPointer? { UnsafeRawPointer(bitPattern: frontend) }
}
