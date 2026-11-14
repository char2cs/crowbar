import SwiftUI

@available(iOS 26.0, *)
struct AppearanceCards: View {
  let mode: String
  let palette: ChromePalette?
  let choose: (String) -> Void
  @Environment(\.colorScheme) private var scheme

  private var ring: Color { palette?.brand(scheme) ?? .accentColor }

  private static let modes: [(id: String, label: String)] = [
    ("light", "Light"), ("dark", "Dark"), ("system", "System"),
  ]

  var body: some View {
    HStack(spacing: 12) {
      ForEach(Self.modes, id: \.id) { item in
        let selected = mode == item.id
        Button {
          choose(item.id)
        } label: {
          VStack(spacing: 6) {
            // PLATFORM: strokeBorder draws inside the shape, over the clipped face: nothing leaves the card.
            Preview(mode: item.id, palette: palette)
              .frame(height: 64)
              .clipShape(.rect(cornerRadius: 10))
              .overlay {
                RoundedRectangle(cornerRadius: 10)
                  .strokeBorder(
                    selected ? ring : Color.secondary.opacity(0.3),
                    lineWidth: selected ? 2.5 : 1
                  )
              }
            Text(item.label)
              .crowbarUI(.footnote)
              .lineLimit(1)
              .minimumScaleFactor(0.8)
              .foregroundStyle(selected ? ring : palette?.secondaryText ?? .secondary)
          }
          .contentShape(.rect)
        }
        .buttonStyle(.plain)
        .accessibilityLabel(item.label)
        .accessibilityAddTraits(selected ? .isSelected : [])
      }
    }
  }

  private struct Preview: View {
    let mode: String
    let palette: ChromePalette?

    var body: some View {
      ZStack {
        switch mode {
        case "light": Face(scheme: .light, brand: lightBrand)
        case "dark": Face(scheme: .dark, brand: darkBrand)
        default:
          Face(scheme: .light, brand: lightBrand)
          Face(scheme: .dark, brand: darkBrand)
            .mask {
              GeometryReader { geometry in
                Path { path in
                  path.move(to: CGPoint(x: geometry.size.width, y: 0))
                  path.addLine(to: CGPoint(x: geometry.size.width, y: geometry.size.height))
                  path.addLine(to: CGPoint(x: 0, y: geometry.size.height))
                  path.closeSubpath()
                }
              }
            }
        }
      }
    }

    private var lightBrand: Color { palette?.brandLight ?? .green }
    private var darkBrand: Color { palette?.brandDark ?? .green }
  }

  private struct Face: View {
    let scheme: ColorScheme
    let brand: Color

    var body: some View {
      let light = scheme == .light
      let ink = light ? Color.black : Color.white
      ZStack(alignment: .topLeading) {
        (light ? Color(white: 0.97) : Color(white: 0.1))
        VStack(alignment: .leading, spacing: 5) {
          Capsule().fill(ink.opacity(0.55)).frame(width: 34, height: 4)
          Capsule().fill(ink.opacity(0.3)).frame(width: 22, height: 4)
        }
        .padding(10)
        Circle().fill(brand).frame(width: 7, height: 7)
          .frame(maxWidth: .infinity, maxHeight: .infinity, alignment: .bottomTrailing)
          .padding(8)
      }
    }
  }
}
