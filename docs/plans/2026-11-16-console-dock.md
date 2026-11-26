# Console dock

Status: tracked follow-up of `2026-11-16-pane-tab-strips.md`; a gate item for parity with develop's console dock. Until it lands the console toggle opens the console as a stackable pane in the focused strip.

## Intended behaviour

The console is a dock that wraps the whole shell. It is not a pane of any tab and is never in the saved layout.

- State: `{ open, dock: top|right|bottom|left, mode: overlay|push, size }`. Defaults: closed, top, overlay, 420 px. Size is clamped to at least 160 px and at most 80 percent of the window axis the dock lies on.
- `ctrl+backtick` toggles it. Escape closes it. In overlay mode a press outside closes it. In push mode the layout area shrinks by the dock size.
- Header: 36 px, with a dock button (cycles top, right, bottom, left) and a mode button (overlay or push).
- A 6 px resize handle on the inner edge. Opening and closing animate for 200 ms, ease-out.
- State persists in the window memory file next to window geometry, not in the layout file.

## Why it is not a small hook

1. The console is a native pane. In overlay mode it floats over native webview holes that sit above GPUI, so the covered holes must be frozen (the frozen layer) or hidden while it is open.
2. Push mode changes the area `shell_view.rs` hands to `visible_panes`; the dock rectangle is carved off before the layout area is computed.
3. Its state needs a field in the `window_state` store.

## Design

- `window_state`: add `ConsoleDock { open, dock, mode, size }` with serde defaults so older files load. A pure `Dock` module owns the cycle order, the size clamp and the carve of a window rectangle into `(dock, rest)`.
- `crowbar_desktop`: `console_dock.rs` holds the live state and the animation clock; `ShellView` asks it for `rest` when it places panes and renders the header and handle above the layout. The console stays a pane of the pane host, placed in the dock rectangle through a `VisiblePane` outside the layout tree.
- `stack_policy`: the console stops being stackable once the dock lands. The toggle sends the dock toggle, never a layout command.

## Tests first

Written to fail before any dock code:

- Dock state: defaults; cycle top, right, bottom, left, top; size clamp at 160 and at 80 percent for each axis; carve of a window rectangle for each dock side in push mode; overlay carves nothing.
- Persistence: a state round trip through the window memory file; a file without the field loads the defaults; the layout file never contains the console.
- Shell: toggle opens and closes; Escape closes; an outside press closes in overlay mode only; push mode shrinks the visible pane bounds; the frozen layer covers native holes under an open overlay.

## Order of work

1. `window_state` field and the pure dock module with the table tests.
2. `ShellView` carve, header, handle and animation, with `shell_view.rs` coordinated with its owner.
3. Remove the console from `stack_policy` and the pane toggle.
4. Live check on Xvfb: toggle, each dock side, both modes, resize, Escape, restart persistence.
