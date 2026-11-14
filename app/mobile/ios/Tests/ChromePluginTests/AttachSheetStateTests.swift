import XCTest

@testable import tauri_plugin_crowbar_chrome

final class AttachSheetStateTests: XCTestCase {
  func testAttachSheetState() throws {
    let state = try decode(
      AttachSheetState.self,
      """
      {"open":true,"title":"Attach files","note":"Up to 25 MB each.",
       "sources":[{"id":"photos","title":"Photos","icon":"Images"},
         {"id":"camera","title":"Camera","icon":"Camera"},{"id":"files","title":"Files","icon":"Folder"}],
       "files":[{"id":"0","name":"a.png","detail":"1 KB · Uploading…","status":"uploading",
         "percent":40,"canRetry":false},
         {"id":"1","name":"b.pdf","detail":"2 MB · offline","status":"bad","percent":null,"canRetry":true}],
       "hint":"Uploading 0 of 1…","attachLabel":"Attach 1 file","canAttach":false}
      """
    )
    XCTAssertEqual(state.files.map(\.percent), [40, nil])
    XCTAssertTrue(state.files[1].canRetry)
    for source in state.sources { XCTAssertNotNil(CrowbarIcon(named: source.icon), source.icon) }
  }
}
