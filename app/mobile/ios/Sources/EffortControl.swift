import SwiftUI

@available(iOS 26.0, *)
struct EffortControl: View {
  let levels: [String]
  let handed: String?
  let text: Color
  let pill: Color
  let onPick: (String) -> Void

  @State private var dial = EffortDial()
  @GestureState private var touch: Int?

  private static let height: CGFloat = 40

  var body: some View {
    let shown = dial.shown(handed: handed).flatMap(levels.firstIndex(of:))
    let lit = touch ?? shown
    GeometryReader { track in
      let width = track.size.width / CGFloat(max(levels.count, 1))
      ZStack(alignment: .leading) {
        if let lit {
          Capsule().fill(pill)
            .frame(width: width, height: Self.height - 6)
            .offset(x: width * CGFloat(lit))
            .animation(.snappy(duration: 0.18), value: lit)
        }
        HStack(spacing: 0) {
          ForEach(Array(levels.enumerated()), id: \.offset) { position, level in
            Text(EffortDial.label(level))
              .font(.subheadline.weight(position == lit ? .semibold : .regular))
              .foregroundStyle(text)
              .lineLimit(1).minimumScaleFactor(0.7)
              .frame(width: width)
          }
        }
      }
      .frame(height: Self.height)
      .contentShape(.rect)
      .gesture(
        DragGesture(minimumDistance: 0)
          .updating($touch) { drag, state, _ in
            state = EffortDial.index(
              at: drag.location.x,
              width: track.size.width,
              count: levels.count
            )
          }
          .onEnded { drag in
            let index = EffortDial.index(
              at: drag.location.x,
              width: track.size.width,
              count: levels.count
            )
            guard levels.indices.contains(index),
              dial.release(on: levels[index], handed: handed)
            else { return }
            onPick(levels[index])
          }
      )
    }
    .frame(height: Self.height)
    .onChange(of: touch) { _, now in if now != nil { Haptics.select() } }
    .onChange(of: handed) { dial.receive(handed: handed, levels: levels) }
    .onChange(of: levels) { dial.receive(handed: handed, levels: levels) }
    .accessibilityElement(children: .ignore)
    .accessibilityLabel("Effort")
    .accessibilityValue(handed.map(EffortDial.label) ?? "")
    .accessibilityAdjustableAction { direction in
      guard let current = dial.shown(handed: handed).flatMap(levels.firstIndex(of:)) else { return }
      let next = current + (direction == .increment ? 1 : -1)
      guard levels.indices.contains(next), dial.release(on: levels[next], handed: handed) else {
        return
      }
      onPick(levels[next])
    }
  }
}
