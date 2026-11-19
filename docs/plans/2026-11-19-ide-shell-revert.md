# IDE shell revert: model and moves

Tree plan section 7 step 6, approved. Spec: develop `web/src/features/panes` (`types/pane.ts`,
`hooks/use-pane-presentation.ts`, `components/pane-container.tsx`) and `features/tabs`.

## Model (`layout/src/ide/`)
- The split leaf becomes develop's `PaneGroup`, held inline in the leaf as the strip `Group` is today
  (no side map keyed by leaf, so nothing can drift):
  `PaneGroup { id: GroupId, chat: Option<PaneId>, editor_tabs: Vec<EditorTab>, active_editor:
  Option<PaneId>, editor_open: bool, chat_selected: bool }`, with
  `EditorTab { pane: PaneId, pinned: bool, preview: bool }`. A leaf with no chat is the stage.
- Every chat and every editor tab stays a `PaneInstance` in the view, so `panehost`, the web views
  and the terminals are untouched. A chat never sits in `editor_tabs`, and a New Tab page never sits
  in `chat` (as in develop).
- Opening, per develop: a chat gets its own leaf. Any other pane joins the focused group's
  `editor_tabs` and becomes active, and `editor_open` is set. The New Tab page is reused per group
  (`find-open-buffer.ts`). This replaces `stack_policy` (app) and the strip `Group` code.
- Focus stays a `PaneId` (the chat or an editor tab); the group is derived from it.
- Schema v3: the v2 leaf group folds into a `PaneGroup` (first chat member = `chat`, the rest =
  `editor_tabs`, active kept). Older files still load.

## Decisions taken during the work
- The strip model is reshaped, not deleted: it is develop's editor-tab behaviour. `ClosePanes` and
  `CloseScope` are the tab context menu's close others / to the right / all, `StripStep` is
  `use-tab-keyboard-nav.ts`, `PinPane`/`UnpinPane` and the pins are `isPinned` (pinned tabs first),
  `OpenPaneInGroup` is `openContent(..., { paneId })` and `MoveToGroup` is a tab drag or reorder.
  These files move to `layout/src/ide/` and act on editor tabs only. Only `remaining.rs`, and
  `tab_groups.rs` and `group_slot.rs` once the presentation geometry replaces them, are deleted.
- `PaneGroup` is held inline in the leaf, with no side map: `{ chat, tabs, active, pinned,
  editor_open, chat_selected }`. Pins are a count of leading tabs, as the strip kept them, not a
  flag per tab; there is no group id and no preview flag until a row needs one.
- Step 1 is two commits: the data shape with no behaviour change (schema v3; older files load as
  the default layout, so there is no migration), then chat and tabs merged into one group together
  with the step-2 presentation geometry.

## Geometry (`layout/src/ide/presentation.rs`, pure)
- Port `use-pane-presentation.ts`: tabs | side | stacked from the leaf size, chat first unless the
  sidebar is on the right. It returns the rects for the top row, the tab bar, the chat, the sash and
  the editor. The app's `holes.rs` places the chat hole and the active editor-tab hole from them.
- The chat/editor ratio is view-local in `ideshell`, as develop keeps it in component state. It is
  not persisted and is not a second copy of anything.

## Views (`ideshell`, new L3 crate: layout, ui, gpui)
- `toprow/`: chat-only header with the `</>` open-editor button, split toggle, review shortcut.
- `tabbar/`: chat tab, editor tab items (glyph, label, close, dirty), "+", overflow, context menu.
  `tab_look` and `path_action` move here from `layoutview`.
- `layoutview` draws pane boxes and the split tree only. A pane's chrome is the `ideshell` element,
  which the app supplies through the existing chrome port.

## Steps (each builds, passes the gates and keeps the app running)
1. `layout`: `ide::PaneGroup` replaces the strip `Group`, with the open, close, activate, reorder,
   pin, `SetEditorOpen` and `SelectChat` commands and the v3 migration. Delete the ten strip model
   files. The app's opening moves from `stack_policy` to the model.
2. `layout::ide::presentation` and the app's holes switch to it.
3. The `ideshell` crate: top row and tab bar, wired as pane chrome; terminals open as editor tabs
   (IDE-042); ctrl+t, ctrl+n, ctrl+w and ctrl+j act on the group.
4. Delete the 21 `layoutview` strip files and `PaneTabs`. Re-pair IDE-040, IDE-041, IDE-042,
   TERM-002 and the PANE and SPL rows from the same chat in both apps.
