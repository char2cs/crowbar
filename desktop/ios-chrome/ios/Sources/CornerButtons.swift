import SwiftUI

/// One top corner's round glass buttons, merged into one glass shape when side by side.
@available(iOS 26.0, *)
struct CornerButtons: View {
  @ObservedObject var model: ChromeModel
  let side: String

  var body: some View {
    GlassEffectContainer(spacing: 8) {
      HStack(spacing: 8) {
        // Only on the chat itself: the IDE, drawers and sheets carry no `ide` button.
        if side == "trailing", model.buttons.contains(where: { $0.id == "ide" }),
          let context = model.context, let percent = context.percent
        {
          ContextMeter(state: context, percent: percent, send: model.send, present: model.present)
        }
        ForEach(model.buttons.filter { $0.side == side }) { button in
          GlassCircle(symbol: button.symbol, label: button.label, enabled: button.enabled) {
            model.send(button.intent.map { .of($0) } ?? .tap(button.id))
          }
        }
      }
    }
  }
}
