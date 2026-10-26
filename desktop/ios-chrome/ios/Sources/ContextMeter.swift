import SwiftUI
import UIKit

/// How full the visible chat's context window is; `percent` nil hides the meter.
struct ContextState: Decodable, Equatable {
  let percent: Double?
  let usedLabel: String
  let canCompact: Bool
  let compacting: Bool
}

/// The chat's context usage as a glass capsule beside the IDE circle; a tap offers compaction.
@available(iOS 26.0, *)
struct ContextMeter: View {
  let state: ContextState
  let percent: Double
  let send: (ChromeIntent) -> Void
  let present: (UIViewController) -> Void

  private var shown: Int { Int(percent.rounded()) }
  private var tint: Color { percent > 95 ? .red : percent > 80 ? .orange : .primary }

  var body: some View {
    Button {
      Haptics.tap()
      present(alert)
    } label: {
      HStack(spacing: 6) {
        Capsule()
          .fill(.primary.opacity(0.2))
          .frame(width: 36, height: 8)
          .overlay(alignment: .leading) {
            // Never thinner than a dot, so a few percent still reads as filled.
            Capsule().fill(tint).frame(width: max(8, 36 * min(1, max(0, percent / 100))))
          }
        if state.compacting {
          ProgressView().controlSize(.mini)
        } else {
          Text("\(shown)%")
            .font(.footnote.weight(.semibold).monospacedDigit())
            .foregroundStyle(tint)
        }
      }
      .padding(.horizontal, 12)
      .frame(height: 44)
      .contentShape(.capsule)
    }
    .buttonStyle(.plain)
    .glassEffect(.regular.interactive(), in: .capsule)
    .accessibilityLabel("Context \(shown) percent used")
    .accessibilityValue(state.compacting ? "Compacting" : "")
    .dynamicTypeSize(...DynamicTypeSize.xxxLarge)
  }

  private var alert: UIAlertController {
    let alert = UIAlertController(
      title: "Context \(shown)% used",
      message: state.compacting ? "Compacting…" : (state.usedLabel.isEmpty ? nil : state.usedLabel),
      preferredStyle: .alert)
    alert.addAction(UIAlertAction(title: "Cancel", style: .cancel))
    let compact = UIAlertAction(title: "Compact", style: .default) { [send] _ in
      send(.of("compact"))
    }
    compact.isEnabled = state.canCompact && !state.compacting
    alert.addAction(compact)
    alert.preferredAction = compact
    return alert
  }
}
