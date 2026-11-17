import XCTest

@testable import tauri_plugin_crowbar_chrome

final class ConnectDraftsTests: XCTestCase {
  private func state(
    name: String,
    address: String,
    code: String
  ) -> ConnectState {
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

  private struct SubmitCase {
    let name: String
    let stage: ConnectStage
    var draftName = ""
    var address = ""
    var code = ""
    let canSubmit: Bool
  }

  private let submitCases = [
    SubmitCase(name: "empty address stage", stage: .address, canSubmit: false),
    SubmitCase(
      name: "blank name",
      stage: .address,
      draftName: "  ",
      address: "lab.example",
      canSubmit: false
    ),
    SubmitCase(
      name: "name and address",
      stage: .address,
      draftName: " lab ",
      address: "lab.example",
      canSubmit: true
    ),
    SubmitCase(name: "fingerprint is always confirmable", stage: .fingerprint, canSubmit: true),
    SubmitCase(name: "code stage needs a code", stage: .code, canSubmit: false),
    SubmitCase(name: "code present", stage: .code, code: "123456", canSubmit: true),
  ]

  func testEachStageSubmitsOnlyWhenItsFieldsAreFilled() {
    for item in submitCases {
      var drafts = ConnectDrafts()
      drafts.name = item.draftName
      drafts.address = item.address
      drafts.code = item.code
      XCTAssertEqual(drafts.canSubmit(at: item.stage), item.canSubmit, item.name)
    }
  }

  func testNamesAreTrimmed() {
    var drafts = ConnectDrafts()
    drafts.name = " lab "
    XCTAssertEqual(drafts.trimmedName, "lab")
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
