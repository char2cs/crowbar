import SwiftUI

final class TabsModel: ObservableObject {
  @Published var state = TabsState(tabs: [], newTabActions: [])
  var send: (ChromeIntent) -> Void = { _ in }
}
