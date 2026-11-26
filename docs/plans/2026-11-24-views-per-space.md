# Views per space

## Why
Switching space (ctrl+shift+2, a switcher mark, a swipe) in develop shows the new space's own
stage: its remembered view, else its most recent view, else the empty stage with the wordmark.
The shell only changes the store's `active_project`. The layout has one window-wide active tab,
so the other space's pane stays on screen while the strip, filtered by space, is empty. Dropping
a chat of the new space onto that pane then merges chats of two spaces into one view.

Rows: TST-034, DND-015, VT-022, ANIM-046 (the tabs half), SPL-012 and SB-161 (re-drive after the
owner moves; bba7cdd3d already refuses), SB-047 and KEY-029 (re-drive: the shortcut is wired by
`project_slot.rs`, but what it shows is wrong). Restart parity depends on UPG-065 (see Risks).

## Develop's model (the spec)
- One window pane store, persisted as one row. `ViewState`
  (`web/src/features/panes/lib/view-state.ts:15`) holds `views: Record<id, ViewRecord>`,
  `viewOrder`, `activeViewId`, `activeViewByProject: Record<projectId, viewId>`,
  `activeProjectId`, one window-wide `stage` (a chatless layout) and the bottom tray.
- A view's space is stored on its record: `ViewRecord.projectId`
  (`web/src/features/panes/types/pane.ts`, "A Recents row: one view, its project, its tiling
  tree"). It is set once, when the record is made: `insertPane` with `{kind:'view', projectId}`
  (`view-ops.ts:88`), `promoteStage` with the active project when a chat lands on the stage
  (`view-ops.ts:69`), `detachPane` with the source record's project. Members are derived from the
  layout's panes, never listed twice.
- The stage shows when `activeViewId` is null: `showingLayout` (`view-state.ts:86`).
  `showView` (`view-state.ts:174`) also records `activeViewByProject[project] = view`.
- Switching space is `paneActions.setActiveProject` (`stores/slices/pane-actions/view-actions.ts:156`),
  driven by the route (`components/layout/use-shell-route-state.ts:84-88`): show the remembered
  view of that project, else keep the showing one if it already belongs there, else
  `nextViewFor(project)` (`view-state.ts:191`: most recently focused pane's view, else band
  order), else null = the stage. On the first call, records with `projectId: ''` (made before the
  window knew its project) join that project.
- Opening another project's chat (`openChat`, `view-actions.ts`) makes its record and remembers it
  in `activeViewByProject` without showing it (`foreign`). `focusPane` on another project's view
  only remembers it (`mayShow`, `view-state.ts:236`).
- Closing the showing record shows `nextViewFor(same project, same workspaces)`, else the stage
  (`dropRecord`, `view-ops.ts:55`). Deleting a space closes its records (`closeViewsForProject`).
- The strip lists `viewOrder` filtered by `projectId === activeProjectId`, or every view with
  `tabListScope: 'all'` (`hooks/use-tab-view-ids.ts`). A click on another space's row routes to
  its chat's workspace, which switches the space (`components/sidebar/lib/recents-actions.ts:13`).
- Law 4, "content never crosses a project": the drop action refuses with "That chat belongs to a
  different space" (`components/sidebar/lib/pane-drop-actions.ts:139-151`); the store itself does
  not check it.
- Hidden views: `SplitViewRoot` keeps every mounted record and the stage mounted under
  `display: none` + `inert`, keyed by id so a switch never remounts (`components/split-view-root.tsx`);
  a hidden pane's native webview is told to hide (`pane-editor-view-content.tsx:136`).
  Workspace stores are retained while a view record holds one of their chats
  (`features/workspace/lib/keep-alive-policy.ts`). A space switch is just another `showView`.
- Persistence: IndexedDB `workspace-layout` row `window`, version 4
  (`lib/persistence/workspace-layout.ts`, schema `lib/persistence/schemas.ts:15-34`) with `views`,
  `viewOrder`, `activeViewId`, `activeViewByProject`, `stage`; debounced 300 ms
  (`stores/window-pane-store.ts:85-103`); repaired on load, dropping pointers whose record is gone
  or of another project (`lib/view-repair.ts:79-81`). The active project is a separate key,
  `crowbar.activeProject` (`core/entities/active-project.ts`).

## The Rust gap
- `layout::Layout` (`app/crates/layout/src/layout.rs`) is `tabs: Vec<Tab>` + `active: Option<TabId>`.
  A `Tab` (= develop's view record) has no space. `active` is window-wide and, by
  `validate_active` (`layout_stored.rs`), may be `None` only when there are no tabs: the shell
  cannot show the stage while any tab exists.
- A tab's space is derived on every read from its chats' projects through the store
  (`shell_view/tab_strip.rs projects_of`, `shell_view/space_drops.rs`). A chatless tab (files,
  terminal, new-tab page) has no space and `TabScope::lists` (`tab_scope.rs`) lists it everywhere.
- The active space is owned by `store` (`live/state.rs active_project`, set by
  `Command::SelectProject` in `sidebar_commands.rs`; `reads::active_project` falls back to the first
  project). Nothing in the layout reacts to it; only `push_tabs` filters the strip.
- Closing a tab hands `active` to its index neighbour of any space (`layout_tabs.rs take_tab_at`).
- Every write that shows a tab sets the single `active` (`tab_commands.rs add/activate`,
  `pane_commands.rs`, `ide/group_commands.rs`).
- Stored form is `{version: 3, tabs, active}`; any other version loads as the default layout
  (`schema.rs`, `layout_stored.rs from_stored`), so a bump without an upcast would drop every
  user's tabs.
- Hidden tabs already behave like develop's parked views: `PaneHost` follows the visible panes of
  the showing tab (`holes.rs visible_panes`, `panehost lifecycle/visibility.rs hide_unlisted`);
  others are parked or kept alive by their type's lifecycle. Nothing is reparented.

## Owner design
Three facts, one owner each; no copy:

| Fact | Owner | Note |
|---|---|---|
| Which space is active | `store` (`State.active_project`) | unchanged; the layout never stores it |
| Which space a tab belongs to | `layout::Tab.space` | set once when the tab is made, as `ViewRecord.projectId` |
| Which tab each space shows | `layout::Layout.shown: BTreeMap<String, TabId>` | absent key = that space's stage; replaces `active` |

- `SpaceId(String)` in the layout crate; `""` is "no space yet", as develop's `projectId: ''`.
- Reads take the space: `Layout::showing(&self, space: &str) -> Option<&Tab>` replaces
  `active_tab()`. No layout command needs the active space: every write that shows a tab writes
  `shown[tab.space]` from the tab's own space. A tab made or focused in a hidden space is
  remembered there without being shown, which is develop's `foreign` branch for free.
- `Command::AddTab { id, title, space }`; `DetachPane`/the new-tab paths copy the source tab's space.
  `ShellState` stamps new tabs from an `ActiveSpace` port (one method, `fn active(&self) -> String`,
  implemented by `store::Store`, a fixed fake in tests), passed to `ShellState::new` as a required
  argument. An opened chat takes its own project when the store knows it (`chat_project`, or a
  draft's `projectId` param), else the active space, as `opts.projectId || activeProjectId`.
- `ShellState::showing()` = `layout.showing(&space.active())`. Every `active_tab()` caller in
  `app/desktop/app/src` (44) and `layoutview` goes through it, so the render, `visible_panes`, focus
  follow, keyboard and pump all see the active space's tab with no write on a switch: a space
  switch is a store change, and `on_data_changed` already refreshes.
- Close: the heir is chosen in the closed tab's own space (neighbour within that space; develop's
  MRU + workspace refinement of `nextViewFor` is a later row if the rig shows it), else the key is
  removed and that space shows its stage.
- Strip: `TabScope::lists` compares `tab.space()` with the active space; `""` tabs are listed
  everywhere until adopted. Scope 'all': activating another space's tab also dispatches
  `SelectProject(tab.space)`, as `focusRecent` routes.
- Drops: `space_drops.rs` reads the target from `tab.space()` instead of deriving it; the dragged
  chat keeps the store's `chat_project`; a dragged view uses its `space`. The refusal stays in the
  shell, where develop keeps it.
- Delete space: where the shell sees a project leave the listing after `DeleteProject`, close the
  tabs whose `space` is that project (develop `closeViewsForProject`), so chatless tabs go too.

## Persistence: v4 and the migration
```json
{ "version": 4,
  "tabs": [ { "id": 3, "title": "", "space": "p1", "panes": {…}, "tree": {…}, "focused": "a" } ],
  "shown": { "p1": 3 } }
```
- `TabRecord`/`TabShape` gain `space`; `LayoutDocument` swaps `active` for `shown`.
  `validate` checks each `shown` entry names a tab of that space; tabs may exist with no entry.
- `schema.rs` keeps "current or default" but gains one upcast: v3 -> v4 sets every tab's `space`
  to `""` and `shown = {"": active}` when `active` is set. Versions other than 3 and 4 still load
  as the default layout.
- Adoption of `""` tabs (develop's first `setActiveProject`): `Command::AdoptSpace { tab, space }`,
  a non-structural command that moves the tab and, if no entry exists for `space`, its `""` shown
  entry. The shell issues it from `on_data_changed` for each `""` tab once it can resolve one:
  the project of its chats when the store knows them (the first chat's, for a legacy tab that
  mixes spaces), else, for a chatless tab, the active space. This is a one-time upgrade of
  legacy records, not a sync: a tab never returns to `""`.
- Restart: `shown` is saved through the existing `LayoutWriter`; each space reopens on its last
  tab. Which space the window opens on is the store's (first project today; UPG-065).

## Slices (each starts red)
1. **Layout owns a tab's space and each space's shown tab** (`app/crates/layout`).
   Red: `tests/model.rs` — `a_tab_shown_in_one_space_is_not_shown_in_another`: AddTab in p1,
   AddTab in p2; `showing("p1")` and `showing("p2")` are each their own tab, `showing("p3")` is
   `None` while tabs exist. Also: closing p1's only tab leaves `showing("p1") == None` and p2
   untouched; FocusPane on a p2 pane does not change `showing("p1")`; `swap_arrangement`
   (withdraw) restores `shown`. Rows: none alone (enables 2-4).
2. **Stored form v4 with the v3 upcast** (`schema.rs`, `layout_stored.rs`, `tab_record.rs`).
   Red: `a_v3_layout_loads_with_its_tabs_unowned_and_shown_in_no_space`: the current v3 fixture
   loads, keeps every tab and pane, every tab's space is `""`, `showing("")` is the old active.
   Plus a v4 round trip and a refused `shown` naming another space's tab. Rows: protects every
   user's saved tabs; UPG rows unchanged.
3. **The shell shows the active space's tab** (`shell_state.rs`, `ActiveSpace` port,
   `shell_view` render/pump/focus, `tab_strip.rs`, `tab_scope.rs`).
   Red (`shell_view` test rig with a fake store of two projects): open a chat of p1, select p2:
   the layout view has no tab (wordmark) and `push_tabs` lists none; select p1 again: the p1 tab
   shows with its focus. Rows: TST-034, DND-015, VT-022, ANIM-046 (tabs), SB-047, KEY-029.
4. **New tabs take their space; opens of another space are remembered, not shown**
   (`opening.rs`, `pane_opener.rs`, `new_tab.rs`, `open_requested.rs`, agent tab opens).
   Red: with p1 active, `DataOutcome::OpenChat` for a p2 chat makes a p2 tab, `showing("p1")`
   unchanged; a file opened on p2's stage makes a p2 tab. Strip click in scope 'all' on a p2 tab
   dispatches `SelectProject(p2)`. Rows: VT-022 ('all' half), DND-015 (drop on the empty stage
   opens the chat as that space's tab).
5. **Drops read the tab's space** (`space_drops.rs`, `tab_strip.rs tab_projects_of` removed).
   Red: dragging a chatless p1 tab onto a p2 tab is refused (today both sides derive no project
   and it passes). Rows: SPL-012, SB-161, TST-034 toast path.
6. **Adopt legacy tabs and close a deleted space's tabs** (`on_data_changed`).
   Red: a loaded v3 layout with a p2 chat tab and a chatless tab, store lists p1 active and knows
   the chat: after one data change the chat tab is p2's, the chatless one p1's, and none is `""`.
   Delete p2: its tabs close, p1's stay. Rows: PRJ-051 stays pass; upgrade of current users.

Each slice deletes what it replaces in the same change (`Layout::active_tab`, `Layout.active`,
`projects_of`, the `projects.is_empty()` arm of `TabScope::lists`).

## Risks
- **Drag between spaces.** bba7cdd3d refuses with develop's toast; slice 5 moves its target side
  to the owner. With scope 'project' a pane of another space is never on screen any more, so the
  only cross-space drags are strip drags under scope 'all' and sidebar rows during a switch
  (DND-100). The layout crate stays permissive like develop's store; the shell is the gate.
- **Undo history.** The shell calls only `withdraw`, not undo/redo. `Arrangement` gains `space` per
  `TabShape` and the `shown` map, so `withdraw` restores the shown tab of the space it opened in.
  Because `ActivateTab`/`FocusPane` are not structural, a withdraw after the user switched tabs in
  another space would restore that space's older entry: `swap_arrangement` restores only the
  `shown` entries of spaces whose tabs the step changed. Test it in slice 1.
- **Hidden panes.** A space switch takes the existing tab-switch path: the leaving tab's panes drop
  out of `visible_panes`, `PaneHost::hide_unlisted` parks or keeps them per their `PaneLifecycle`,
  exactly as develop parks a view under `display: none` and hides its webview. No reparenting, no
  new keep-alive rule. `tab_cap` eviction counts every space's tabs, as develop's retention
  counts every record; confirm on the rig that a terminal in a hidden space keeps its scrollback.
- **Chatless tabs.** Develop's stage is one window-wide layout and is never a strip row; the shell
  makes a chatless tab a strip row. Giving it the space it was made in keeps law 4 and the strip
  honest; the only visible difference is a file left on develop's stage following you across
  spaces. Record it on the rig in slice 4; if the owner wants the global stage, it becomes a
  `""`-space tab shown when a space has no entry, with no format change.
- **Restart lands on the first space** until the store persists `active_project` (UPG-065). The
  per-space memory survives either way; this plan does not add a copy of the active project.
- **Drafts.** A draft's chat is unknown to the store; its params carry `projectId`
  (`2026-11-24-draft-thread.md`), which slice 4 reads.
