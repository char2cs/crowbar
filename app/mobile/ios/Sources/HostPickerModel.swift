import SwiftUI

final class HostPickerModel: ObservableObject {
  @Published var state = HostPickerState(open: false, list: .empty)
  var send: (ChromeIntent) -> Void = { _ in }
}
