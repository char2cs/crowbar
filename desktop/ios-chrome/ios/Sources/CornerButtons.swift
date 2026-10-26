import SwiftUI

/// Which screen's header a corner belongs to.
enum Header { case chat, ide }

/// One top corner's round glass buttons of a screen's header, merged into one glass shape when
/// side by side.
@available(iOS 26.0, *)
struct CornerButtons: View {
  @ObservedObject var model: ChromeModel
  let header: Header
  let side: String

  private var buttons: [ChromeButton] { header == .chat ? model.headers.chat : model.headers.ide }

  var body: some View {
    GlassEffectContainer(spacing: 8) {
      HStack(spacing: 8) {
        // Only on the chat itself: the IDE carries no `ide` button.
        if side == "trailing", buttons.contains(where: { $0.id == "ide" }),
          let context = model.context, let percent = context.percent
        {
          ContextMeter(state: context, percent: percent, send: model.send, present: model.present)
        }
        ForEach(buttons.filter { $0.side == side }) { button in
          GlassCircle(symbol: button.symbol, label: button.label, enabled: button.enabled) {
            model.send(button.intent.map { .of($0) } ?? .tap(button.id))
          }
        }
      }
    }
  }
}
