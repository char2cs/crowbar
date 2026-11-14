import UIKit
import XCTest

@testable import tauri_plugin_crowbar_chrome

@available(iOS 26.0, *)
final class SheetPresenterTests: XCTestCase {
  func testClosingWithNothingPresentedFinishesAtOnce() {
    let presenter = SheetPresenter(
      undimmed: false,
      content: { UIViewController() },
      onUserDismiss: {}
    )
    var finished = false
    presenter.closeThen { finished = true }
    XCTAssertTrue(finished)
  }

  func testOpeningWithoutARootBuildsNoContent() {
    var built = 0
    let presenter = SheetPresenter(
      undimmed: false,
      content: {
        built += 1
        return UIViewController()
      },
      onUserDismiss: {}
    )
    presenter.update(.open, over: nil)
    XCTAssertEqual(built, 0)
  }
}
