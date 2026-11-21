# Chat zoom is the window's, kept by the shell

## Problem

Develop keeps one chat zoom for the window (`features/window/stores/zoom-store.ts`): `mod+=`,
`mod+-` and `mod+0` change it wherever focus is, through a window `keydown` listener
(`features/keymaps/hooks/use-zoom-keyboard.ts`) that `WorkspaceView` mounts. Every chat surface
reads it. Reset also sets the editor and terminal zoom back to 100%. Nothing persists it.

The shell has no chat zoom at all (ZOOM: KEY-022, KEY-023, KEY-024, TERM-076, WIN-034, WIN-035):
- Pane pages do not mount `WorkspaceView`, so no page listens for the chords.
- Each chat pane is its own webview with its own `useZoomStore`, so zoom set in one chat would not
  follow to the next chat tab, as develop's does.
- With the terminal (native) focused, the chord never reaches a page at all (TERM-076).
- The keymap says panes keep zoom chords for themselves (`keymap/src/lib.rs`), and
  `ShellCommand::parse("agent.zoomIn")` is `None` by test.

## Owner

The shell owns the chat zoom level, the same way it owns the theme and the breakpoints: one value
per window, never written back by a page. Pages render it and keep no copy anyone can edit.

## Steps

1. **Keymap.** `agent.zoomIn`, `agent.zoomOut` and `agent.zoomReset` become shell commands with
   develop's chords (`mod+=`, `mod+-`, `mod+0`). They run *alongside* the focused view
   (`ShellCommand::runs_alongside`, `Keymap::alongside`): the shell hears them in the capture phase
   and does not stop the key, so a focused terminal still zooms its own font (develop's terminal
   handler only calls `preventDefault`, so both of develop's window listeners fire). `resolve`
   never returns them. Pages forward them as `command.run`, like `mod+t`. Key repeat is ignored.
2. **Shell state.** `ShellState::zoom_chat` steps the chat zoom in percent, 30 to 300 in steps of
   10, reset to 100; not persisted. The terminal's zoom stays the persisted `terminalFontSize`
   (develop's `zoomBy(±2)`); develop never sets `editorZoomLevel` or `terminalZoomLevel`, so the
   shell has no editor or terminal level.
3. **Protocol and panehost.** `ShellFrame::ChatZoom { percent }` (`chat.zoom`), sent on mount and
   once per change; `PaneHost` keeps the last value sent and is the one place it lives.
4. **Page.** `PaneClient.onChatZoom`; pane-boot's `followChatZoom` feeds it to
   `useZoomStore.setZoom`, which `agent-chat-pane.tsx` already reads.

## Tests

- keymap: the three chords run alongside, `resolve` skips them, and pages forward them.
- shell state: in, out, reset, clamped at 30 and 300; a chat mounted after a zoom gets it.
- shell view: `mod+=` and `mod+0` with the terminal focused change the chat zoom.
- panehost: sent on mount and once per change.
- SDK and page: `onChatZoom` hears the frame; `followChatZoom` sets the zoom store.
- Rig: the six ZOOM rows.
