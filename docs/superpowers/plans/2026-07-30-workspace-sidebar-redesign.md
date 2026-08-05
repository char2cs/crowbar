# Workspace sidebar redesign — implementation plan

Executes `docs/superpowers/specs/2026-07-30-workspace-sidebar-redesign-design.md`.
Written to be run by subagents: each wave states its files, its acceptance
criteria, and the command that proves it. Do not start a wave until the
previous wave's gate is green.

## Two findings from exploration that change the spec

**1. `Workspace.ParentID` is doing two jobs.** It is both the fork-point lineage
(`api/internal/domain/workspace.go:20-64`) and the sidebar tree edge. Putting a
folder id in it silently breaks three things that resolve it back to a
workspace: `ResolveMergeEligibility` (`merge_eligibility.go:37-71`) returns a
zero value rather than an error, so a foldered workspace looks permanently
unmergeable with no diagnostic; `summaryBase` (`worktree.go:306-325`) reads the
parent's *branch* for the diff base and a folder has none; `guardReparent` /
`childHasChildren` call `workspaces.Get` on the id and error out.

**Decision: folders get their own field.** `Workspace.ParentID` keeps meaning
fork parent, untouched. Add `Workspace.FolderID string`, set only on a
fork-root; descendants inherit their folder from their fork ancestor. This is
what the spec's own rule ("a folder may not split a fork chain") already
implies, and it leaves every git path alone.

**2. "One flat list across every project" is not a sidebar change.** The whole
app is single-active-project from the transport up:
`components/app-sync-provider.tsx:49-129` tears down and rebuilds the repo and
workspace WS subscriptions on every `activeProjectId` change, and
`buildScopedRepoTree` (`lib/store/build-repo-tree.ts:126-136`) hard-filters to
that one project. Rendering every project means subscribing to every project's
repo + workspace streams concurrently and reworking that scoping model.

This is the single largest item in the plan and it was not visible when the
spec was written. It is Wave 3 below, and it is the one place where the work is
architectural rather than presentational.

## Performance is an acceptance criterion, not a footnote

Crowbar's mission includes being fast. Every wave below is expected to leave the
sidebar **quicker than it found it**, and a wave that cannot show that is not
done. Concretely:

- **Subscribe to what is visible** (Wave 3). Cost scales with what is on screen,
  not with how much work you have.
- **Incremental merge, not whole-tree rebuild** on every WS frame (Wave 3).
- **Never trade the visible artifact for CPU.** Rows, counts and status stay
  correct and immediate; the savings come from not subscribing to what is not
  shown, never from showing less.
- **Keep the drag path off React state.** The ghost is positioned through a ref
  today and must stay that way; `hoverTargetId` re-renders only on boundary
  crossings.
- **Memoise per repo, and keep the deps honest.** `rootsByRepo` exists so a drag
  does not rebuild every node graph; new inputs (folders, order) must join its
  dependency array or reorders silently will not paint.
- **Animate cheaply.** The collapse tween is 120ms on one wrapper per section,
  not per row, and is skipped entirely under `prefers-reduced-motion`.

## Verification commands

`bun` is not on PATH, and **`bunx tsc` outside `web/` runs an unrelated npm
package called `tsc` (v7.0.2) and exits 0** — a false green. Always `cd web`
first and use one of:

```
cd web && ./node_modules/.bin/tsc --noEmit                      # typecheck
cd web && PATH="$HOME/.bun/bin:$PATH" bun run test              # vitest
cd web && PATH="$HOME/.bun/bin:$PATH" bun run lint              # eslint + prettier
cd web && PATH="$HOME/.bun/bin:$PATH" bun run doctor            # react-doctor
cd api && make test                                             # go test -race
cd api && make test-integration                                 # black-box, -p 1
```

Live: `make dev-desktop` (never touch prod Crowbar), then the Tauri MCP tools.
Reuse a running dev instance — `pgrep -fl crowbar-api` first; stacked launches
orphan daemons fighting one socket.

---

## Wave 0 — guard rails

Pin the behaviour the refactor must not break, **before** touching anything.

- `web/src/__tests__/components/layout/workspace-tree-roles.test.tsx` — assert
  the `role="tree"` / `role="treeitem"` / `role="group"` contract on repo rows
  and workspace rows. No test asserts this today; the contract lives only in a
  code comment (`workspace-tree.tsx:69-74`) and regressing it silently
  reintroduces the bug it was written to fix.
- `api/tests/regressions_test.go` — pin that reparent still refuses a non-leaf
  (`ErrChildHasChildren` → 409) and that delete still broadcasts the `deleted`
  tombstone.

**Gate:** both suites green, no production code changed.

---

## Wave 1 — backend: Folder entity

Folder is a **GORM CRUD store**, not an event-sourced aggregate. It has no
OCC-sensitive concurrent write path, no worktree, and no reactor; the aggregate
machinery (own event log, snapshot store, 8-way shard router) would be pure
cost. AutoMigrate adds the table with no migration code.

Files:

- `api/internal/domain/folder.go` — new. `{ ID, RepoID, ProjectID, ParentID,
  Name, Order int }` with `TableName() "folders"`. `ParentID` is a workspace id,
  a folder id, or "" for repo root.
- `api/internal/domain/workspace.go` — add `FolderID string
  \`json:"folderId,omitempty"\``. No migration: the read model is a JSON blob
  (`store/projections/store.go:26-33`), so old rows replay with the zero value,
  exactly as `Kind` documents.
- `api/internal/app/gorm.go` — register the store alongside Repositories.
- `api/internal/app/usecases/folder/` — new package: Create, Rename, Move,
  Delete, ListInRepo. Delete reparents children to the folder's own parent
  rather than cascading (a folder holds no worktrees; deleting the workspaces
  under it is a separate, explicit act).
- Guards, mirroring `guardReparent`'s shape: `ErrFolderCycle` (a folder may not
  move under its own descendant), `ErrFolderCrossRepo`, `ErrFolderNotEmpty`
  where relevant.
- `api/internal/api/libs/status.go` — wire every new sentinel into
  `isConflict`/`isNotFound`/`isBadRequest`. This is a flat `errors.Is` chain,
  not a registry; a sentinel that is not added here falls through to a generic
  500.

**Gate:** `cd api && make test` green.

---

## Wave 2 — backend: ordering, API, streams

**Ordering.** Add `Order int` to `domain.Workspace` (JSON blob, no migration),
`domain.Repository` and `domain.Project` (GORM AutoMigrate adds the column,
existing rows default to 0).

There is no `ORDER BY` anywhere in the storage layer — `Store.FindAll` issues a
bare `.Find` (`store.go:160-168`) and the workspace read model has no sortable
column. **Sort at a single choke point per entity** and use it from both the
REST list handler and the WS `Snapshot` function, or the two will disagree and
the sidebar will reorder itself on reconnect. Add the sort helpers next to the
DTO converters so both paths import the same one.

**API** (per the spec's table): folder CRUD under
`/v0/projects/:p/repos/:r/folders`, `order` on the workspace/repo/project
PATCHes, `projectId` on the repo PATCH.

**Streams.** Folder rides path A (the GORM pattern): the mutating handler calls
`hub.BroadcastFolder` directly after the save, mirroring
`repos.go:532-539`. Needs `dto.FolderDTO` + `FolderDTOFrom`,
`WebSocketHub.BroadcastFolder` + `PushFolder`, a `foldersDef` +
`folderSnapshot` (namespace `projectID/repoID/id`), and a
`*ws.Broadcaster[dto.FolderDTO]` on `v0.Container`.

**Invariant.** Extend `guardReparent` so a move that would separate a workspace
from its fork parent is refused — including via a folder move. Server-side, not
only in the UI.

**Gate:** `cd api && make test && make test-integration` green, including new
`TestRegression_*` for: folder CRUD, folder nested under a protected branch,
reparent refused when it would split a fork chain, order dense after every
move, repo moved between projects keeps its workspaces.

---

## Wave 3 — frontend: multi-project data layer, subscribed lazily

**The architectural wave, and the one with a performance budget.** Nothing
renders differently yet; this only makes every project's data available.

Crowbar is meant to be fast. The naive reading of "show every project" —
subscribe every project's repo and workspace streams at boot — is strictly
worse than today: N× WebSocket subscriptions, N× snapshot replays on every
reconnect, N× IndexedDB writes, and a full sidebar rebuild triggered by a frame
from a project you cannot even see. It would make the sidebar slower in exact
proportion to how much you use Crowbar.

**Subscribe to what is visible, not to what exists.** The collapse affordance
in the design makes this natural, and it makes collapsing a project genuinely
cheaper rather than merely tidier:

| stream | when subscribed |
|---|---|
| `/v0/projects` | always — one stream, a handful of rows |
| a project's repos | while that project is expanded, or it is the active one |
| a repo's workspaces | while that repo is expanded, or it holds the active workspace |

Subscription cost becomes proportional to what is on screen. A collapsed
project costs one row of cached name and nothing else. Expanding subscribes;
collapsing tears down after a short grace period so a collapse/expand tap does
not thrash the socket.

Rows for a collapsed project come from the entity cache, so they render
instantly and offline — no spinner, no layout shift on expand. This is a
latency win, not a degradation: nothing visible is withheld, because a
collapsed project shows only its own row by definition.

**Second performance fix, mandatory in this wave.** `rebuildSidebar()`
(`app-sync-provider.tsx:32-41`) currently answers *every* WS frame by re-reading
the entire entity cache and replacing the whole tree (`setRepos`). That is
O(all workspaces) per frame today and would be O(all projects × all workspaces)
after this change. Replace it with an incremental merge keyed by the frame's
entity id, and keep the full rebuild only for the initial seed. `mergeRepos` and
`applyWorkspaceDTO` already exist in `sidebar.ts:312-375`, are already tested,
and have no production caller — they are the intended path and were never wired
up.

Files: `components/app-sync-provider.tsx`, `lib/store/build-repo-tree.ts`,
`lib/store/sidebar.ts`, `lib/store/workspace-list.ts`.

Watch: `EMPTY_PROJECTS` (`lib/store/projects.ts:7-13`). Any new selector with an
empty fallback needs a module-level stable constant, never an inline `?? []`,
or React throws "Maximum update depth exceeded" somewhere unrelated.

**Gate:** typecheck + full vitest green; the sidebar still renders exactly as
before (this wave is invisible by design); and a measured check that expanding a
project subscribes exactly one repo stream, collapsing tears it down, and a
workspace frame no longer triggers a whole-tree rebuild.

---

## Wave 4 — frontend: tree, rows, layout

- `workspace-tree-utils.ts` — `buildWorkspaceTree` becomes folder-aware and
  ordered. `WorkspaceTreeNode` gets a `kind` discriminator; keep passing every
  `Workspace` field through untouched (an existing test asserts `toEqual(full)`).
  Keep the existing cycle detection and extend it to folder edges.
- `workspace-tree.tsx` — all projects in one scroll, `<hr>` between, repos at
  the project's own indent. **Keep the outer
  `<div className="flex flex-1 flex-col overflow-hidden">` exactly as the single
  flex child**, or the carousel's scroll-snap math breaks
  (`sidebar-carousel.tsx:43-96`).
  `rootsByRepo`'s `useMemo` deps must grow to include folders and order, or
  reorders silently will not re-render.
- New `folder-row.tsx` — `role="treeitem"`, `aria-expanded`, children in a fresh
  `role="group"`, built from `ROW_BASE` (never hand-rolled styles).
- `workspace-tree-item.tsx` — hover-only `+` (`display`, not opacity), counts as
  a second line inside an unchanged 36px row, one 16px glyph box for every
  leading glyph.
- `project-home-row.tsx` — glyph⇄chevron swap, glyph folds the tree, row opens
  project home, drop the "Switch project" button.
- `repo-section.tsx` — row opens repo home; collapse stays on the chevron.
- Retire `project-switcher-panel.tsx` and its `sidebar-nav` push.

**Gate:** typecheck, vitest, lint, doctor green.

---

## Wave 5 — frontend: drag

`workspace-tree-context.tsx`. `findDropTarget` (L109-120) is the single
definition of a droppable target; extend it with `data-folder-drop` and
`data-project-drop`, and return a `{ id, mode }` rather than a string so the
before/after/into decision is made once.

- Thresholds: folder rows 20/60/20, workspace rows 30/40/30.
- Two signals, never both: hairline + end-cap for reorder, filled row for nest.
- The indicator must not lie — under an expanded parent, the gap below its own
  row means *first child*.
- Locked rows reorder among siblings only; no nest affordance while dragging one.
- Ghost becomes a clone of the row, up to 3 stacked + count badge, still
  positioned imperatively through the ref (never React state).
- Edge scroll within 36px of either end.
- New context fields go in the correct `useMemo`: drag-time state in
  `dragValue`, callbacks in `actionsValue`, or the re-render split that exists
  to avoid a storm is defeated.

Tests must stub `HTMLElement.prototype.setPointerCapture` and
`document.elementsFromPoint` — jsdom has neither, and `setup.ts` does not stub
them. Follow `pane-sash.test.tsx` for the MouseEvent-as-pointer technique.

**Gate:** typecheck, vitest (including the full drop matrix and every refusal),
lint, doctor green.

---

## Wave 6 — frontend: keep set, tray, animation

- **Keep set** — two independent states. Multiselection is transient, drawn like
  the open row, cleared by a plain click. The keep set is a snapshot taken at
  collapse, carries no styling, and survives both clearing the selection and
  navigating. Generalise the existing `showChildrenSection` escape hatch
  (`workspace-tree-item.tsx:72-80`) rather than adding a parallel mechanism, and
  give `repo-section.tsx:238` the same escape hatch it currently lacks.
- **Tray** — a genuinely new non-destructive removal path. Do not reuse the
  trash branch: `performDeleteWorkspace` is destructive with no revert. Rows in
  the tray are ordinary rows. Repos wait on Cancel/Remove with no clock.
  Retire `workspace-tree-footer.tsx`'s trash zone.
- **Animation** — 0.12s `cubic-bezier(0.42, 0, 0.58, 1)` on height + opacity.
  Each collapsible section needs its own wrapper so there is one box to close;
  clear the inline height when the expand finishes or the box can never grow
  again. Respect `prefers-reduced-motion`.

**Gate:** typecheck, vitest, lint, doctor green.

---

## Wave 7 — live verification

`make dev-desktop`, then drive the Tauri MCP tools. Never verify against prod
Crowbar. Screenshot each:

1. Create a folder from a row's `+` with a trailing slash; nest one under a
   protected branch.
2. Reorder within a level; nest; reorder a repo; move a repo between projects;
   reorder projects.
3. Drag a protected branch — confirm no nest affordance and a refused drop.
4. ⌘-click two rows, collapse the parent, navigate away, confirm both kept;
   fold them away from the parent's control.
5. Drag a workspace to the editor; undo from the tray; let one drain.
6. Drag a repo to the editor; Cancel; then Remove.
7. Collapse/expand at every level and watch the animation.
8. Restart the app and confirm folders, order and collapse all survived.

**Gate:** every step verified live with a screenshot, plus all suites green.

---

## Sequencing note

Waves 1–2 (backend) and Wave 3 (FE data layer) can run in parallel — they share
no files. Waves 4–6 are sequential: they all touch
`workspace-tree-context.tsx`, `workspace-tree-item.tsx` and `sidebar.ts`, and
parallel agents on those will conflict.

## Before certifying

Re-check for sibling agent sessions in this worktree (`ps -eo pid,etime,command
| grep -i claude`) and for commits on this branch that are not ours. Two such
commits (`a40f7135`, `221f3d5b`) landed here from another session before this
work started.
