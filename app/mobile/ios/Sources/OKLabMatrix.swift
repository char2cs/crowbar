import Foundation

// SPEC: https://bottosson.github.io/posts/oklab/ OKLab matrices (Ottosson): OKLab to cube-root LMS, and LMS to linear sRGB.
enum OKLabMatrix {
  static let lmsFromLab: [[Double]] = [
    [1, 0.3963377774, 0.2158037573],
    [1, -0.1055613458, -0.0638541728],
    [1, -0.0894841775, -1.2914855480],
  ]
  static let linearRGBFromLms: [[Double]] = [
    [4.0767416621, -3.3077115913, 0.2309699292],
    [-1.2684380046, 2.6097574011, -0.3413193965],
    [-0.0041960863, -0.7034186147, 1.7076147010],
  ]

  static func apply(
    _ matrix: [[Double]],
    to vector: [Double]
  ) -> [Double] {
    matrix.map { row in zip(row, vector).map { $0 * $1 }.reduce(0, +) }
  }
}
