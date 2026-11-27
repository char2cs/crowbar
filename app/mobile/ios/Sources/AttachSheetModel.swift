import SwiftUI

final class AttachSheetModel: ObservableObject {
  @Published var state = AttachSheetState(
    open: false,
    title: "",
    note: "",
    sources: [],
    files: [],
    hint: nil,
    attachLabel: "",
    canAttach: false
  )
}
