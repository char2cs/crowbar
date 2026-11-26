# terminalview

## Owns

Drawing one terminal and turning gpui input into `terminal` calls: grid painting (text, cursor,
highlights, scrollbar), keys, IME and the pointer, selection, the search bar, and the channel pair
that joins a view to its daemon session.

## Public interface

- `TerminalView`: `new`, `focus`, `set_settings`, `run_after_attach`, `grid_size`; it emits
  `Event`.
- `Settings`: the theme and font a view draws with.
- `Link` (the view end) and `Peer` (the host end) of the daemon channel.

## Depends on

- `terminal`: the terminal model.
- `ui`: the design system.
- `gpui`.
