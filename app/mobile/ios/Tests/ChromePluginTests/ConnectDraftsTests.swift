import XCTest

@testable import tauri_plugin_crowbar_chrome

final class ConnectDraftsTests: XCTestCase {
  private func state(name: String, address: String, code: String) -> ConnectState {
    ConnectState(
      open: true,
      stage: .address,
      name: name,
      address: address,
      code: code,
      fingerprint: "",
      busy: false,
      error: nil
    )
  }

  func testDraftsStartFromTheWebsState() {
    let drafts = ConnectDrafts(from: state(name: "lab", address: "lab.example", code: "123"))
    XCTAssertEqual([drafts.name, drafts.address, drafts.code], ["lab", "lab.example", "123"])
    let empty = ConnectDrafts(from: nil)
    XCTAssertEqual([empty.name, empty.address, empty.code], ["", "", ""])
  }

  func testAddressStageNeedsANameAndAnAddress() {
    var drafts = ConnectDrafts()
    XCTAssertFalse(drafts.canSubmit(at: .address))
    drafts.name = "  "
    drafts.address = "lab.example"
    XCTAssertFalse(drafts.canSubmit(at: .address))
    drafts.name = " lab "
    XCTAssertTrue(drafts.canSubmit(at: .address))
    XCTAssertEqual(drafts.trimmedName, "lab")
  }

  func testFingerprintStageIsAlwaysConfirmableAndCodeStageNeedsACode() {
    var drafts = ConnectDrafts()
    XCTAssertTrue(drafts.canSubmit(at: .fingerprint))
    XCTAssertFalse(drafts.canSubmit(at: .code))
    drafts.code = "123456"
    XCTAssertTrue(drafts.canSubmit(at: .code))
  }

  func testSubmissionIsTheIntentOfTheStage() {
    var drafts = ConnectDrafts()
    drafts.name = "lab"
    drafts.address = "lab.example"
    drafts.code = "123456"
    let address = drafts.submission(at: .address).payload
    XCTAssertEqual(address["type"] as? String, "submitAddress")
    XCTAssertEqual(address["name"] as? String, "lab")
    XCTAssertEqual(address["address"] as? String, "lab.example")
    XCTAssertEqual(
      drafts.submission(at: .fingerprint).payload["type"] as? String,
      "confirmFingerprint"
    )
    let code = drafts.submission(at: .code).payload
    XCTAssertEqual(code["type"] as? String, "submitCode")
    XCTAssertEqual(code["code"] as? String, "123456")
  }
}
