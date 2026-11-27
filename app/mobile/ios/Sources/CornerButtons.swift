import SwiftUI

@available(iOS 26.0, *)
struct CornerButtons: View {
  @ObservedObject var model: ChromeModel
  let header: Header
  let side: String

  private var buttons: [ChromeButton] { header == .chat ? model.headers.chat : model.headers.ide }

  var body: some View {
    GlassEffectContainer(spacing: Spacing.small) {
      HStack(spacing: Spacing.small) { circles }
    }
  }

  private var circles: some View {
    ForEach(buttons.filter { $0.side == side }) { button in
      CornerCircle(button: button, model: model)
    }
  }

  private struct CornerCircle: View {
    let button: ChromeButton
    @ObservedObject var model: ChromeModel

    var body: some View {
      GlassCircle(
        icon: button.icon,
        label: button.label,
        enabled: button.enabled,
        tint: model.palette?.text,
        action: press
      )
    }

    private var press: () -> Void {
      { model.send(button.intent.map { .of($0) } ?? .tap(button.id)) }
    }
  }
}
