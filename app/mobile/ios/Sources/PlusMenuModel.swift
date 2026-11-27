import SwiftUI

final class PlusMenuModel: ObservableObject {
  @Published var state = PlusMenuState(open: false, title: "", items: [])
}
