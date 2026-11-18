# viewtabs

## Owns

The window's view-tab row: one pill per view, reordering and merging by drag, the close menu and
the sidebar toggle (`mod+b`), driven by the layout model's commands.

## Public interface

- `TabBar`: `new`, `with_clock`, `set_tabs`, `set_sidebar`; it emits `Event` (`Apply`,
  `ToggleSidebar`).
- `TabView`, `MemberView`, `MemberKind`: what one pill shows.
- `SidebarState`, `Clock`.
- `ShellLook`, `Span`, `StripDrag`, `TabDrop`, `NewTab` (exported today; the host does not use
  them).
- `tabsgallery` binary (`src/bin/gallery/`): the row over a fake model, with chat chips to drag in.

## Depends on

- `layout`: the tabs and the commands a drag applies.
- `protocol`: pane and chat ids.
- `ui`: the design system.
- `gpui`.
