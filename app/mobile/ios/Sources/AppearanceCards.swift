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
    HStack(spacing: Spacing.medium) { cards }
  }

  private var cards: some View {
    ForEach(Self.modes, id: \.id) { item in
      Card(
        item: item,
        selected: mode == item.id,
        palette: palette,
        ring: ring,
        choose: choose
      )
    }
  }

  private struct Card: View {
    let item: (id: String, label: String)
    let selected: Bool
    let palette: ChromePalette?
    let ring: Color
    let choose: (String) -> Void

    var body: some View {
      Button(action: { choose(item.id) }) { label }
        .buttonStyle(.plain)
        .accessibilityLabel(item.label)
        .accessibilityAddTraits(selected ? .isSelected : [])
    }

    private var label: some View {
      VStack(spacing: Spacing.snug) {
        preview
        title
      }
      .contentShape(.rect)
    }

    // PLATFORM: ios: strokeBorder draws inside the shape, over the clipped face: nothing leaves the card.
    private var preview: some View {
      Preview(mode: item.id, palette: palette)
        .frame(height: Metric.previewHeight)
        .clipShape(.rect(cornerRadius: Corner.preview))
        .overlay { border }
    }

    private var border: some View {
      RoundedRectangle(cornerRadius: Corner.preview)
        .strokeBorder(
          selected ? ring : Color.secondary.opacity(Alpha.dim),
          lineWidth: selected ? Metric.selectedBorder : 1
        )
    }

    private var title: some View {
      Text(item.label)
        .crowbarUI(.footnote)
        .lineLimit(1)
        .minimumScaleFactor(Metric.titleShrink)
        .foregroundStyle(selected ? ring : palette?.secondaryText ?? .secondary)
    }
  }

  private struct Preview: View {
    let mode: String
    let palette: ChromePalette?

    var body: some View {
      ZStack { faces }
    }

    @ViewBuilder private var faces: some View {
      switch mode {
      case "light": Face(scheme: .light, brand: lightBrand)
      case "dark": Face(scheme: .dark, brand: darkBrand)
      default: split
      }
    }

    @ViewBuilder private var split: some View {
      Face(scheme: .light, brand: lightBrand)
      Face(scheme: .dark, brand: darkBrand).mask { GeometryReader { Diagonal(size: $0.size) } }
    }

    private var lightBrand: Color { palette?.brandLight ?? .green }
    private var darkBrand: Color { palette?.brandDark ?? .green }
  }

  private struct Diagonal: View {
    let size: CGSize

    var body: some View {
      Path { path in
        path.move(to: CGPoint(x: size.width, y: 0))
        path.addLine(to: CGPoint(x: size.width, y: size.height))
        path.addLine(to: CGPoint(x: 0, y: size.height))
        path.closeSubpath()
      }
    }
  }

  private struct Face: View {
    let scheme: ColorScheme
    let brand: Color

    private var light: Bool { scheme == .light }
    private var ink: Color { light ? Color.black : Color.white }
    private var ground: Color {
      light ? Color(white: Metric.lightGround) : Color(white: Metric.darkGround)
    }

    var body: some View {
      ZStack(alignment: .topLeading) {
        ground
        lines
        dot
      }
    }

    private var lines: some View {
      VStack(alignment: .leading, spacing: Spacing.chip) {
        Capsule().fill(ink.opacity(Alpha.ink))
          .frame(width: Metric.longLine, height: Metric.lineHeight)
        Capsule().fill(ink.opacity(Alpha.dim))
          .frame(width: Metric.shortLine, height: Metric.lineHeight)
      }
      .padding(10)
    }

    private var dot: some View {
      Circle().fill(brand).frame(width: Metric.statusDot, height: Metric.statusDot)
        .frame(maxWidth: .infinity, maxHeight: .infinity, alignment: .bottomTrailing)
        .padding(Spacing.small)
    }
  }
}
