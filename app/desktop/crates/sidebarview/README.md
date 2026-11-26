# sidebarview

Layer L3 (gpui view). See `docs/rust-style.md` (crate layers).

## Owns

The `Sidebar` gpui entity: it frames the content view and draws the header, space switcher, mode
bar, the Chats, Files and Changes panels, menus, dialogs, peek card, toasts and row drags, and
turns input into `sidebar` behaviour calls and `Command`s. Rendering details that only the views
read stay here even when they are gpui-free (pixel scale, menu model, drag state).

## Public interface

- `Sidebar`: `new(data, sink, content, (preferences, cx))`, `toggle`, `set_mode`, `dock_to`,
  `preferences`, `ask_to_discard`, `toast`, `clear_toast`, `set_space_folder`, `begin_peek`,
  `end_peek`, `preview_drag`, and `Sidebar::toast_side(DockSide)`: the toast edge for a dock edge
  (the shell's floating notices use it).
- `Event`: what the entity tells the shell.

## Depends on

- `sidebar`: the data contract and the behaviour the views draw.
- `ui`: the design system (theme, icons, menus, toasts, motion).
- External: `gpui`, `tracing`.
