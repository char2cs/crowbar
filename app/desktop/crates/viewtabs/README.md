# viewtabs

## Owns

The window's view-tab row: one pill per view, reordering and merging by drag, the close menu and
the sidebar toggle (`mod+b`), driven by the layout model's commands.

## Public interface

- `TabBar`: `new`, `set_tabs`, `set_sidebar`, `has_tabs`, `preview`, `pane_drop`; it emits `Event`
  (`Apply`, `ToggleSidebar`, `DragOut`, `DropOut`, `DragCancel`, `DropChat`).
- `TabView`, `MemberView`, `MemberKind`, `MemberGlyph`: what one pill shows.
- `SidebarState`, `TabDrop`.

## Depends on

- `layout`: the tabs and the commands a drag applies.
- `protocol`: pane and chat ids.
- `ui`: the design system.
- `gpui`.
