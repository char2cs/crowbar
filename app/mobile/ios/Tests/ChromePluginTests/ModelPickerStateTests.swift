import XCTest

@testable import tauri_plugin_crowbar_chrome

final class ModelPickerStateTests: XCTestCase {
  func testModelPickerState() throws {
    let state = try decode(
      ModelPickerState.self,
      """
      {"open":true,"providers":[{"id":"provider-a","name":"Provider A","fallbackIcon":"Robot",
        "models":[{"id":"m1","name":"Model 1"}]}],
       "selected":{"providerId":"provider-a","modelId":"m1"},"efforts":["low","high"],"effort":null}
      """
    )
    XCTAssertNil(state.providers[0].icon)
    XCTAssertEqual(state.providers[0].models[0].name, "Model 1")
    XCTAssertEqual(state.selected, PickerSelection(providerId: "provider-a", modelId: "m1"))
    XCTAssertEqual(state.efforts, ["low", "high"])
  }

  func testModelPickerStateCarriesThePermissionSection() throws {
    let state = try decode(
      ModelPickerState.self,
      """
      {"open":true,"providers":[],"selected":null,"efforts":[],"effort":null,
       "permission":{"levels":[
         {"value":"guarded","label":"Guarded","description":"Asks first."}],
        "level":"guarded"}}
      """
    )
    XCTAssertEqual(state.permission?.levels.map(\.label), ["Guarded"])
    XCTAssertEqual(state.permission?.levels.map(\.description), ["Asks first."])
    XCTAssertEqual(state.permission?.level, "guarded")
  }

  func testFilteredKeepsProvidersWithAMatchingModelOrName() {
    let alpha = PickerProvider(
      id: "a",
      name: "Alpha",
      icon: nil,
      fallbackIcon: "Robot",
      models: [PickerModel(id: "1", name: "Fast"), PickerModel(id: "2", name: "Deep")]
    )
    let beta = PickerProvider(
      id: "b",
      name: "Beta",
      icon: nil,
      fallbackIcon: "Robot",
      models: [PickerModel(id: "3", name: "Fast")]
    )
    let state = ModelPickerState(
      open: true,
      providers: [alpha, beta],
      selected: nil,
      efforts: [],
      effort: nil,
      permission: nil
    )
    for item in filterCases {
      let shown = state.filtered(by: item.query)
      XCTAssertEqual(shown.map(\.id), item.providerIds, item.query)
      XCTAssertEqual(shown.first?.models.map(\.id) ?? [], item.firstModelIds, item.query)
    }
  }

  private struct FilterCase {
    let query: String
    let providerIds: [String]
    let firstModelIds: [String]
  }

  private let filterCases = [
    FilterCase(query: "  ", providerIds: ["a", "b"], firstModelIds: ["1", "2"]),
    FilterCase(query: "deep", providerIds: ["a"], firstModelIds: ["2"]),
    FilterCase(query: "BETA", providerIds: ["b"], firstModelIds: ["3"]),
    FilterCase(query: "zzz", providerIds: [], firstModelIds: [])]
}
