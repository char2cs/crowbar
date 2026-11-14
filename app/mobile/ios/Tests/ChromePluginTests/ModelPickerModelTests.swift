import UIKit
import XCTest

@testable import tauri_plugin_crowbar_chrome

final class ModelPickerModelTests: XCTestCase {
  private func dataURL() -> String {
    let png = UIGraphicsImageRenderer(size: CGSize(width: 1, height: 1))
      .pngData { context in
        UIColor.black.setFill()
        context.fill(CGRect(x: 0, y: 0, width: 1, height: 1))
      }
    return "data:image/png;base64," + png.base64EncodedString()
  }

  private func state(icon: String?) -> ModelPickerState {
    ModelPickerState(
      open: true,
      providers: [
        PickerProvider(
          id: "p",
          name: "P",
          icon: icon,
          fallbackIcon: "Robot",
          models: []
        )
      ],
      selected: nil,
      efforts: [],
      effort: nil,
      permission: nil
    )
  }

  func testProviderIconIsDecodedFromItsDataURL() {
    let model = ModelPickerModel()
    model.apply(state(icon: dataURL()))
    XCTAssertNotNil(model.icons["p"])
    XCTAssertTrue(model.state.open)
  }

  func testUnreadableIconLeavesTheFallback() {
    let model = ModelPickerModel()
    model.apply(state(icon: "data:image/png;base64,@@@"))
    XCTAssertNil(model.icons["p"])
    model.apply(state(icon: nil))
    XCTAssertNil(model.icons["p"])
  }
}
