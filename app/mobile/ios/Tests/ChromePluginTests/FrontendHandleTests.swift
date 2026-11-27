import Foundation
import XCTest

@testable import tauri_plugin_crowbar_chrome

final class FrontendHandleTests: XCTestCase {
  private func decode(_ json: String) throws -> FrontendHandle {
    try JSONDecoder().decode(FrontendHandle.self, from: Data(json.utf8))
  }

  func testDecodesTheAddressRustSends() throws {
    let handle = try decode(#"{"frontend":4096}"#)
    XCTAssertEqual(handle.pointer, UnsafeRawPointer(bitPattern: 4096))
  }

  func testAZeroAddressIsNoFrontend() throws {
    let handle = try decode(#"{"frontend":0}"#)
    XCTAssertNil(handle.pointer)
  }
}
