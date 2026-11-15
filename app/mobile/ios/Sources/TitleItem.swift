import SwiftUI

@available(iOS 26.0, *)
struct TitleItem: ToolbarContent {
  let title: String

  var body: some ToolbarContent {
    ToolbarItem(placement: .principal) {
      Text(title).crowbarHeading(size: Metric.pageTitle, relativeTo: .title2)
    }
  }
}
