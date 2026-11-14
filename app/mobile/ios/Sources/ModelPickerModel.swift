import SwiftUI
import UIKit

final class ModelPickerModel: ObservableObject {
  @Published var state = ModelPickerState(
    open: false,
    providers: [],
    selected: nil,
    efforts: [],
    effort: nil,
    permission: nil
  )
  private(set) var icons: [String: UIImage] = [:]
  var send: (ChromeIntent) -> Void = { _ in }

  func apply(_ next: ModelPickerState) {
    for provider in next.providers where icons[provider.id] == nil {
      icons[provider.id] = provider.icon.flatMap(Self.image)
    }
    state = next
  }

  private static func image(dataURL: String) -> UIImage? {
    guard let comma = dataURL.firstIndex(of: ","),
      let data = Data(base64Encoded: String(dataURL[dataURL.index(after: comma)...]))
    else { return nil }
    return UIImage(data: data, scale: 3)
  }
}
