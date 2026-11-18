# terminal

## Owns

Emulation of one PTY stream (an alacritty grid fed by daemon frames), the render snapshot the view
draws, selection, search and links in the text, and every byte encoding toward the program: keys,
mouse reports, paste and dropped paths.

## Public interface

- `Terminal`: `apply`, `snapshot`, `resize` and the encode, scroll, search and select methods;
  `Config`, `Event`, `Palette`, `sanitize_title`.
- Wire: `DaemonFrame` (what the daemon sends) and `ClientMessage` (what goes back).
- Snapshot model: `Snapshot`, `Row`, `Segment`, `StyleSpan`, `StyleFlags`, `Underline`,
  `BackgroundSpan`, `CursorCell`, `CursorShape`, `Highlight`, `Rgb`.
- Input values: `KeyInput`, `Modifiers`, `MouseEvent`, `MouseAction`, `MouseButton`,
  `MouseTracking`, `ScreenBuffer`, `SelectionKind`, `ClickTracker`, `ScrollAccumulator`,
  `ScrollbarThumb`, `WheelRoute`, `route_wheel`, `format_dropped_paths`.
- Geometry: `CellMetrics`, `CellHit`, `CellSide`, `DragEdge`, `ViewPoint`, `cells_f32`, `floor_cells`.
- Links and search: `Link`, `LinkTarget`, `resolve_file_path`, `SearchOptions`, `SearchQuery`,
  `SearchSummary`.
- `Error`.

## Depends on

No workspace crate: alacritty_terminal only. No gpui, no I/O.
