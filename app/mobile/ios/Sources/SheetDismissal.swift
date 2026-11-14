enum SheetDismissal {
  case allowed, blocked

  init(isBlocked: Bool) { self = isBlocked ? .blocked : .allowed }
}
