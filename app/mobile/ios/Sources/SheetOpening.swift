enum SheetOpening {
  case open, closed

  init(isOpen: Bool) { self = isOpen ? .open : .closed }
}
