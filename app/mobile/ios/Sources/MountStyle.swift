enum MountStyle {
  case intrinsicHidden, intrinsicShown, fillHidden

  var sized: Bool { self != .fillHidden }
  var hidden: Bool { self != .intrinsicShown }
}
