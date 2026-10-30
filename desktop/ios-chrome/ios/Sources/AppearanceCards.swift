import SwiftUI

/// Light, Dark and System as three small previews of the app, the chosen one outlined.
struct AppearanceCards: View {
  let mode: String
  let palette: ChromePalette?
  let choose: (String) -> Void

  private static let modes: [(id: String, label: String)] = [
    ("light", "Light"), ("dark", "Dark"), ("system", "System"),
  ]

  var body: some View {
    HStack(spacing: 12) {
      ForEach(Self.modes, id: \.id) { item in
        let selected = mode == item.id
        Button { choose(item.id) } label: {
          VStack(spacing: 6) {
            // strokeBorder draws inside the shape, over the clipped face: nothing leaves the card.
            Preview(mode: item.id, palette: palette)
              .frame(height: 64)
              .clipShape(.rect(cornerRadius: 10))
              .overlay {
                RoundedRectangle(cornerRadius: 10)
                  .strokeBorder(selected ? Color.accentColor : Color.secondary.opacity(0.3),
                    lineWidth: selected ? 2.5 : 1)
              }
            Text(item.label)
              .crowbarUI(.footnote)
              .lineLimit(1)
              .minimumScaleFactor(0.8)
              .foregroundStyle(selected ? Color.accentColor : palette?.secondaryText ?? .secondary)
          }
          .contentShape(.rect)
        }
        .buttonStyle(.plain)
        .accessibilityLabel(item.label)
        .accessibilityAddTraits(selected ? .isSelected : [])
      }
    }
  }

  /// A mock window: two lines of text and a dot, on a light, dark or diagonally split surface.
  private struct Preview: View {
    let mode: String
    let palette: ChromePalette?

    var body: some View {
      ZStack {
        switch mode {
        case "light": face(light: true)
        case "dark": face(light: false)
        default:
          face(light: true)
          face(light: false).mask(Diagonal())
        }
      }
    }

    // Each card's dot is the brand of the mode it previews; the split card's dot sits on its dark half.
    private var lightBrand: Color { palette?.brandLight ?? .green }
    private var darkBrand: Color { palette?.brandDark ?? .green }

    private func face(light: Bool) -> some View {
      let ink = light ? Color.black : Color.white
      return ZStack(alignment: .topLeading) {
        (light ? Color(white: 0.97) : Color(white: 0.1))
        VStack(alignment: .leading, spacing: 5) {
          Capsule().fill(ink.opacity(0.55)).frame(width: 34, height: 4)
          Capsule().fill(ink.opacity(0.3)).frame(width: 22, height: 4)
        }
        .padding(10)
        Circle().fill(light ? lightBrand : darkBrand).frame(width: 7, height: 7)
          .frame(maxWidth: .infinity, maxHeight: .infinity, alignment: .bottomTrailing)
          .padding(8)
      }
    }
  }

  /// The lower-right half of the card.
  private struct Diagonal: Shape {
    func path(in rect: CGRect) -> Path {
      var path = Path()
      path.move(to: CGPoint(x: rect.maxX, y: rect.minY))
      path.addLine(to: CGPoint(x: rect.maxX, y: rect.maxY))
      path.addLine(to: CGPoint(x: rect.minX, y: rect.maxY))
      path.closeSubpath()
      return path
    }
  }
}
