import Foundation

enum Motion {
  static let keyboardDuration: TimeInterval = 0.25
  static let keyboardCurve: UInt = 7
  static let curveShift: UInt = 16
  static let throwSeconds = 0.15
  static let maxFrameStep = 1.0 / 30
  static let stepsPerSecond = 240.0
  static let atRestVelocity = 0.05
  static let settleTolerance = 1e-9
  static let rampFactor = 4.0
  static let shadowAlpha = 0.18
  static let minFrameRate: Float = 60
  static let maxFrameRate: Float = 120
  static let swipeBias = 1.2
}
