import SwiftUI

@available(iOS 26.0, *)
struct EffortControl: View {
  let levels: [String]
  let handed: String?
  let text: Color
  let pill: Color
  let onPick: (String) -> Void

  fileprivate static let height: CGFloat = 40

  var body: some View {
    GeometryReader { track in
      Track(
        levels: levels,
        handed: handed,
        text: text,
        pill: pill,
        width: track.size.width,
        onPick: onPick
      )
    }
    .frame(height: Self.height)
  }

  private struct Track: View {
    let levels: [String]
    let handed: String?
    let text: Color
    let pill: Color
    let width: CGFloat
    let onPick: (String) -> Void

    @State private var dial = EffortDial()
    @GestureState private var touch: Int?

    private var shown: Int? { dial.shown(handed: handed).flatMap(levels.firstIndex(of:)) }
    private var lit: Int? { touch ?? shown }
    private var segment: CGFloat { width / CGFloat(max(levels.count, 1)) }

    var body: some View {
      ZStack(alignment: .leading) {
        highlight
        labels
      }
      .frame(height: EffortControl.height)
      .contentShape(.rect)
      .gesture(drag)
      .onChange(of: touch, feedback)
      .onChange(of: handed, receive)
      .onChange(of: levels, receive)
      .accessibilityElement(children: .ignore)
      .accessibilityLabel("Effort")
      .accessibilityValue(handed.map(EffortDial.label) ?? "")
      .accessibilityAdjustableAction(adjust)
    }

    @ViewBuilder private var highlight: some View {
      if let lit {
        Capsule().fill(pill)
          .frame(width: segment, height: EffortControl.height - Metric.pillInsetY)
          .offset(x: segment * CGFloat(lit))
          .animation(.snappy(duration: Metric.snapDuration), value: lit)
      }
    }

    private var labels: some View {
      HStack(spacing: 0) { levelTexts }
    }

    private var levelTexts: some View {
      ForEach(Array(levels.enumerated()), id: \.offset) { position, level in
        Text(EffortDial.label(level))
          .font(.subheadline.weight(position == lit ? .semibold : .regular))
          .foregroundStyle(text)
          .lineLimit(1).minimumScaleFactor(Metric.labelShrink)
          .frame(width: segment)
      }
    }

    private var drag: some Gesture {
      DragGesture(minimumDistance: 0)
        .updating($touch, body: track)
        .onEnded(release)
    }

    private var track: (DragGesture.Value, inout Int?, inout Transaction) -> Void {
      { drag, state, _ in
        state = EffortDial.index(at: drag.location.x, width: width, count: levels.count)
      }
    }

    private var release: (DragGesture.Value) -> Void {
      { drag in
        guard
          let level = dial.pick(at: drag.location.x, width: width, levels: levels, handed: handed)
        else { return }
        onPick(level)
      }
    }

    private var feedback: (Int?, Int?) -> Void {
      { _, now in
        guard now != nil else { return }
        Haptics.select()
      }
    }

    private var receive: () -> Void {
      { dial.receive(handed: handed, levels: levels) }
    }

    private var adjust: (AccessibilityAdjustmentDirection) -> Void {
      { direction in
        let delta = direction == .increment ? 1 : -1
        guard let level = dial.step(by: delta, levels: levels, handed: handed) else { return }
        onPick(level)
      }
    }
  }
}
