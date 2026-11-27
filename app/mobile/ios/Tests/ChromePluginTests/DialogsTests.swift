import XCTest

@testable import tauri_plugin_crowbar_chrome

final class DialogsTests: XCTestCase {
  func testDialogOptions() throws {
    let prompt = try decode(
      Dialogs.PromptOptions.self,
      #"{"title":"t","value":"v","placeholder":"p","confirmLabel":"OK"}"#
    )
    XCTAssertNil(prompt.message)
    XCTAssertThrowsError(try decode(Dialogs.ConfirmOptions.self, #"{"title":"t"}"#))
  }
}
