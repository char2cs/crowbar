# One overlay rule over native web views

Web views draw above anything GPUI paints. Supersedes `2026-11-17-overlays-above-web-views.md` and
the one-X11-window-per-overlay path (`platform/src/linux/float`, `ui/src/floating/layers`).

## The rule
1. Menus, selects, confirms, file choosers: native widgets behind `webview::Desktop`.
2. Anything typed into: drawn in the main window; the web views it covers are hidden while open
   (a modal with dim hides all).
3. Drags: all web panes hidden, each pane area tinted with `secondary` inside its border; ghost and
   drop zones drawn in the main window.
4. Pane handle: drawn by layoutview; its rect is cut out of the web view (X SHAPE on Linux; macOS
   leaves the view whole until a layer mask + hit-test spike runs on a Mac).
5. Tooltips and toasts: pop-up windows that never take focus or keys.

## Inventory
| Floating thing | Where | Rule |
|---|---|---|
| Context menus, dropdowns, selects (checked item) | `ui/floating/menu/trigger/*`, `native_menu.rs`; GTK in `webview/.../linux/menu_popping.rs` | 1 |
| Confirms: reload/unsaved, kill/detach, delete space/discard, delete file, revoke host/clear data | `ui/floating/native_alert.rs` -> `app/src/native/alert.rs` | 1 |
| File choosers | `app/src/native/file_chooser.rs` | 1 |
| Settings, shell dialogs (name, file, reload, unsaved, branch import, remote folders, folder import, pairing, question) | `settingsview`, `app/src/shell/dialogs`, `branchimport`, `folderpicker`, `folderimport`, `pairing` via `ShellView::floating` | 2, all |
| Halt gate, first run / legacy migration (oobe), backend crash-loop panel | `startup/src/view.rs`, `oobe`, `app/src/daemon/backend/crash_panel.rs` | 2, all |
| Drawn `ui::Dialog` (fallback confirm) | `ui/floating/dialog.rs` | 2, all |
| Sidebar confirm, file dialog, question, narrow drawer + scrim | `sidebarview/src/overlays/dialogs`, `panels/explorer/file_dialog_view.rs`, `sidebar/layout.rs` | 2, all |
| Popovers (space icon, row mark, create space), sidebar row menu, peek card, terminal link menu, drawn menus (macOS until step 7) | `ui/floating/popover.rs`, `popup.rs`, `sidebarview/src/overlays/{popup,peek}`, `app/.../terminal_links.rs` | 2, covered panes |
| Drag ghosts (tab, pane grip, sidebar row, tab preview), drop preview, sidebar drop line | `ui/floating/{drag_ghost,tab_preview}.rs`, `ideshell/tabbar/drag`, `viewtabs/tab_bar/ghost.rs`, `layoutview/.../drag`, `sidebarview/.../reorder` | 3 |
| Pane handle | `layoutview/src/layout_view/pane/{pane_cell,pane_handle}.rs` | 4 |
| Tooltips, toasts (shell, sidebar), readout | `ui/floating/tooltip.rs`, `ui/display/toast*`, `sidebarview/src/overlays/toasts.rs`, `app/src/daemon/backend/crash_panel.rs` | 5 |

OS file drags onto a page stay with the page (`platform::XdndProxy`); nothing is hidden for them.

## State owner
`ui::Cover`, one per window: the open inline overlays, each keyed by its portal's element state
(a `Slot`, released with it) and holding its kind and the cards it painted. It is derived from what
is open, never set by hand, and survives cached views because the entry lives as long as the
portal's element state. It answers `Covered`: the whole window (a modal or a drag is open) or the
cards of the open popovers. `ShellView` observes it and syncs; `PaneHost::sync` sets each visible
pane `Covered` touches to `Visibility::Covered` (stage and lifecycle untouched) and keeps native
focus in the shell while anything covers. On Linux a covered view stays mapped with all of it cut
away (X SHAPE), since unmapping and mapping it again shows ~100 ms of black. A press on a page runs the open click-away layers. The same `Cover` tells
layoutview a drag is live (tint).

The handle's hole is owned by `LayoutView::handle_hole` (hovered pane of a split tab); a change
emits `LayoutEvent::Hovered`, `ShellView` passes it with `Covered` as `panehost::Overlays`, and
each pane's web view is cut through `webview::ViewCutout`; the GTK side re-cuts only on a change.

## Dead once the rule holds
- Float windows for `Modal`, `Popover`, `Drag`: `layers/drag_forward.rs`, `Root`'s keyboard hand-back,
  owner focus taking and activation holding, `Owner::follow_activation`, `holds_keys`,
  `Float::takes_keys`, `FloatHost::focus`, `FloatPlacer::pin` / `focus`, `Part::Catch` in float windows,
  the `Dialog`/`Floating` window kinds in `opening.rs`.
- This plan's predecessor doc.

Kept: `WindowActivity` (a web view holding the keyboard keeps the window active for toasts and
carets), the passive float window (tooltips, toasts) and its X shaping, and the drawn menus on
macOS: `NSMenu` needs `unsafe` (target objects, `performBlock`), which the lane may not allow.
Confirms are only the desktop's alert (GTK, `NSAlert`); one that cannot be asked is logged and
counts as cancelled.
X reports a float window cut away whole as fully obscured and GPUI then draws no frame for it, so
its `Root` sends it an Expose whenever its owner's layers or `WindowActivity` change.
