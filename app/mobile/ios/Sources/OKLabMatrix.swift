import Foundation

enum OKLabMatrix {
  private static let longFromA = 0.3963377774
  private static let longFromB = 0.2158037573
  private static let mediumFromA = -0.1055613458
  private static let mediumFromB = -0.0638541728
  private static let shortFromA = -0.0894841775
  private static let shortFromB = -1.2914855480
  private static let redFromLong = 4.0767416621
  private static let redFromMedium = -3.3077115913
  private static let redFromShort = 0.2309699292
  private static let greenFromLong = -1.2684380046
  private static let greenFromMedium = 2.6097574011
  private static let greenFromShort = -0.3413193965
  private static let blueFromLong = -0.0041960863
  private static let blueFromMedium = -0.7034186147
  private static let blueFromShort = 1.7076147010

  private static let longRow: [Double] = [1, longFromA, longFromB]
  private static let mediumRow: [Double] = [1, mediumFromA, mediumFromB]
  private static let shortRow: [Double] = [1, shortFromA, shortFromB]
  private static let redRow: [Double] = [redFromLong, redFromMedium, redFromShort]
  private static let greenRow: [Double] = [greenFromLong, greenFromMedium, greenFromShort]
  private static let blueRow: [Double] = [blueFromLong, blueFromMedium, blueFromShort]

  static let lmsFromLab: [[Double]] = [longRow, mediumRow, shortRow]
  static let linearRGBFromLms: [[Double]] = [redRow, greenRow, blueRow]

  static func apply(
    _ matrix: [[Double]],
    to vector: [Double]
  ) -> [Double] {
    matrix.map { row in zip(row, vector).map { $0 * $1 }.reduce(0, +) }
  }
}
