# Workspace sidebar redesign

Branch `feature/better-workspace-sidebar`. The visual and interaction design is
**closed**: build the interface as it stands in the prototype
(https://claude.ai/code/artifact/bbcd1385-d449-4b00-860b-8c9c6fb706ec). This
document records what that prototype settled, so the refactor has something
exact to execute against.

Every value below was measured in the prototype. Use them rather than
re-deriving; several were arrived at by discarding a plausible alternative.

## Goals

1. **Folders.** Organise workspaces without lying about lineage.
2. **Manual order**, at every level of the sidebar.
3. **Keep rows visible through a collapse**, so a folded group can still show
   what you are living in.
4. **Remove by dragging into the editor**, with a holding tray.
5. **One flat list across every project**, retiring the nested project switcher.
6. **Width back to the branch name**, which is what the sidebar is for.

## Non-goals

- **No filter/search field.** ⌘K already finds branches and switches
  workspaces; a second way to find things in the sidebar would duplicate it.
- **No re-opening the project row question.** The project row stays a row.
- No changes to the Chats, Files or Git panels.
- No migration code for stale persisted state — pre-production, so a stale
  layout falls back gracefully and is rewritten on next use.

## Data model

Four things gain a persisted order, and one entity is new.

### Folder (new)

```
Folder {
  id        string
  repoId    string     // folders are repo-scoped
  parentId  string     // a workspace id, a folder id, or the repo id
  name      string
  order     int        // index within parentId
  collapsed bool       // per-user UI state
}
```

A folder is organisation only: no branch, no git status, no worktree, no
`WorkspaceStatus`. It may be a child of anything a workspace may be a child of,
**including a protected branch** — that is where most of them will live, hanging
off `develop`.

**Invariant, enforced server-side and not merely by the UI that happens to
prevent it: a folder may not split a fork chain.** Moving a workspace into a
folder moves its descendants with it. Reject a reparent that would separate a
workspace from its fork parent.

### Ordering

- `Workspace.order` — index within `parentId`.
- `Folder.order` — as above, same sibling space as workspaces.
- `Repo.order` — index within its project.
- `Project.order` — index within the sidebar.

Sibling order is a dense index rebuilt on every move; there is no fallback sort
rule once a user has ordered a level.

### Home destinations

Both already exist and both must be reachable from the sidebar:

- **Project home** — the project's non-git directory (`/ide/$projectId/home`).
- **Repo home** — the repo's default workspace (`repo.defaultWorkspaceId`),
  which the repo header row already represents.

## API

| Endpoint                                  | Purpose                                                                 |
| ----------------------------------------- | ----------------------------------------------------------------------- |
| `POST /projects/:p/repos/:r/folders`      | create; body `{ name, parentId }`                                       |
| `PATCH /projects/:p/repos/:r/folders/:f`  | rename, reparent, reorder                                               |
| `DELETE /projects/:p/repos/:r/folders/:f` | delete; children reparent to the folder's own parent                    |
| `PATCH .../workspaces/:w`                 | gains `order`; existing reparent path enforces the fork-chain invariant |
| `PATCH /projects/:p/repos/:r`             | gains `projectId` (move between projects) and `order`                   |
| `PATCH /projects/:p`                      | gains `order`                                                           |

Folders ride the existing repos/workspaces WS streams so the sidebar updates
without refetching.

## Interaction

### Drag and drop

One pointer-based system (the app's is already pointer-based, in
`workspace-tree-context.tsx`), three movable classes that do not mix:

| dragging                  | onto                           | result                                        |
| ------------------------- | ------------------------------ | --------------------------------------------- |
| workspace / folder        | workspace or folder, same repo | before / after / into                         |
| workspace / folder        | repo header, same repo         | into (append at root)                         |
| repo                      | repo                           | before / after (moves project if they differ) |
| repo                      | project row                    | into                                          |
| project                   | project                        | before / after                                |
| workspace / folder / repo | editor pane                    | remove                                        |
| anything else             | anything else                  | refused, no indicator drawn                   |

**Thresholds.** The outer 20% of a folder row reorders, the middle 60% nests.
Workspace rows use a 30% edge band, because nesting under one re-parents a fork
and that is the heavier action.

**Two signals, never both.** A 2px line with a circle end-cap marks a reorder; a
filled row marks a nest.

**The line must not lie.** The gap under an _expanded_ parent's own row is the
slot before its first child, so dropping there makes the row that parent's first
child. Placing after the whole subtree is reached from the top edge of whatever
follows it. Do not move the indicator to the end of the subtree instead — it
would leave the marker nowhere near the pointer.

**Protected branches reorder, never re-parent.** A locked row may move among its
own siblings and nothing else; the nest affordance never appears while one is
being dragged, and "after" an expanded row is refused because that would nest it.

**The drag ghost is a clone of the grabbed row**, up to three stacked at
`translate(i*4px, i*4px)` with the ones behind at opacity 0.25, plus a count
badge when more than one. Today's differently-styled chip is the thing being
replaced.

**Edge scroll** while the pointer is within 36px of either end of the scroller,
or a drag from the last project to the first is impossible without letting go.

### Removal

Drag onto the editor pane. The pane shows a dashed destructive overlay naming
what will go.

- **Workspaces and folders** land in a tray at the sidebar's foot as _ordinary
  rows_ — same 36px box, same glyph, same mono label — with a hairline draining
  along the bottom edge and the seconds left beside it. The delete fires when it
  drains (8s). The figures are written to the DOM from one timer for the whole
  tray, never held in state: the bar says roughly how long and costs no renders
  to say it, and the numeral must cost no more.
- **A repo** lands in the same tray but runs no clock: it takes every worktree
  under it, so it waits with `Cancel` / `Remove` until answered.
- **Projects are not removable this way.** The pane does not offer the overlay.

Guard worth adding: arm the delete only once the pointer has been inside the
pane for a beat. The pane is a large target immediately beside the sidebar and a
long reorder drag will cross it.

### Keep rows through a collapse

This is **two independent states**. Conflating them is the bug to avoid, and it
is not obvious — it produces symptoms that look like styling problems.

1. **Multiselection** — ⌘/ctrl-click toggles, ⇧-click ranges. Drawn _exactly_
   like the open workspace; there is no third treatment. **A plain click clears
   it.** That, not the styling, is what stops several lit rows accumulating.
2. **The keep set** — a snapshot taken at the moment a parent collapses, from
   whatever was multiselected or open. **It carries no styling at all.**
   Clearing the multiselection and opening an unrelated workspace both leave it
   untouched; a kept row must never evaporate because you navigated.

Kept rows render one indent step under the collapsed parent, whatever depth they
really live at. A kept row brings its whole subtree — keeping a parent while
dropping children you can already see would be the odd behaviour.

What marks the state is the **parent**, never the kept rows: its folder icon
shows three dots while it is closed but still holding rows, and hovering it
reveals a fold-away control that releases everything it holds. That control is
the only way to dismiss the set wholesale; ⌘-clicking a row off removes it
individually.

### Rows

- **Project row** — the glyph and the collapse chevron share one slot and hover
  swaps which is `display: none`. Clicking the glyph folds the project's whole
  tree; clicking anywhere else opens project home. No trailing chevron. The
  chevron stays visible while collapsed, or a folded project is a dead end.
- **Repo row** — keeps `import branches`, `+`, and a trailing collapse chevron.
  Clicking the row opens repo home. The glyph is _not_ swapped here: a repo
  avatar carries identity, and a git status glyph carries state, so trading
  either for a chevron would cost information the project's glyph does not.
- **Workspace / folder rows** — trailing `+` and, when they have children, a
  chevron. The `+` leaves the flow until the row is hovered or focused.
- **Bulk actions live in the context menu**, not a selection bar: group into a
  folder, remove N. Escape clears, Delete removes. Grouping makes the folder at
  once under a default name and opens its rename editor with that name selected
  — the gesture is "these belong together", and a modal asking for a name before
  anything happens puts the naming ahead of the grouping.
- **Renaming** — double-click the name. One editor and one `renamingId` for the
  whole tree; the id says whether the commit is a branch rename or a folder
  PATCH. A locked row offers none of it; a folder, which locks nothing and holds
  no branch, offers all of it.
- **Creating** — the row's `+` opens one inline input. A **trailing slash means
  folder**; anything else is a workspace. No second button, no right-click-only
  path. Placeholder: `branch-name, or name/ for a folder`.
- The standing `New` row is gone. At three levels of nesting it stacked three
  deep at different indents with nothing saying which level you were adding to.

### Layout

- All projects in one scroll, separated by an `<hr>`.
- Repos sit at the **project's own indent**. The first indent step belongs to a
  repo's workspaces.
- One `Import project` row closes the list.

## Visual

|                           | value                                                                                                                                                                                                                                            |
| ------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| row                       | unchanged `ROW_BASE` — 36px, `rounded-lg`, `mx-1.5 my-0.5`, 13px/500                                                                                                                                                                             |
| selected + open           | both wear `ROW_ACTIVE`; there is no third treatment                                                                                                                                                                                              |
| every leading glyph       | **16px box** (repo avatar and the project slot are 20px)                                                                                                                                                                                         |
| change counts             | a second line **under** the branch name, 10.5px. The _line_ is `--muted-foreground`; the `+`/`-` figures keep their own green/red tokens, taken down on the light theme. A `+` and a `-` one pixel wide at this size are not enough on their own |
| active row height         | still 36px — the label rises, the row does not grow                                                                                                                                                                                              |
| collapse / expand         | **0.12s, `cubic-bezier(0.42, 0, 0.58, 1)`** on `height` + `opacity`                                                                                                                                                                              |
| icon⇄chevron swap         | `transform 0.1s, opacity 0.15s`; `rotate(90deg)` open, `rotate(0deg)` collapsed                                                                                                                                                                  |
| row indent transition     | `margin-inline-start 0.1s ease-in-out`                                                                                                                                                                                                           |
| drop indicator            | moves at `0.05s ease-out`                                                                                                                                                                                                                        |
| folder icon open/close    | **no transition** — open and closed are two glyphs from the shared icon family, swapped. At a 16px box the difference between them is a few pixels of shear, so a tween spends its whole duration saying what the swap says at once              |
| trailing action appearing | `opacity 120ms ease`, as a keyframe on appear: the control leaves the FLOW at rest (`display`), which is what returns its 24px box and the row's 6px gap to the branch name, and `display` cannot transition                                     |

Rules the prototype learned the hard way:

- **One label position per level.** A glyph box differing by 2px between row
  types puts a visible wobble down the left edge wherever they interleave. Every
  leading glyph shares one box size.
- **Muted token, never a faded foreground.** `workspace-row-base.ts` already
  documents why: a transparency composites differently over the sidebar glass,
  the hover accent and the raised active surface.
- **Hide by removing from flow, not by fading.** A hidden-but-present button
  still holds its 24px box _and_ its 6px flex gap, so `opacity: 0` returns no
  width at all.
- **The active row must not change height.** If it grows for its second line,
  every row beneath it shifts each time you switch workspaces.

## Frontend shape

Touched: `workspace-tree.tsx`, `workspace-tree-item.tsx`,
`workspace-tree-context.tsx`, `workspace-tree-utils.ts`, `repo-section.tsx`,
`project-home-row.tsx`, `workspace-row-base.ts`, `drag-ghost.tsx`,
`workspace-tree-footer.tsx`, `lib/store/sidebar.ts`.

Retired: `project-switcher-panel.tsx` and its nav-stack entry.

Per `CLAUDE.md`: kebab-case files, PascalCase components, narrow store
selectors, `getState()` only in handlers and effects, no `components/` imports
from stores, tests in `web/src/__tests__/` mirroring the source tree.

Each collapsible section needs its own wrapper element, so the collapse
animation has a single box to close rather than N rows to orchestrate; the
inline height must be cleared when an expand finishes or the box can never grow
again.

## Testing

Frontend, in `web/src/__tests__/`:

- the drop matrix above, including every refusal
- the indicator and the drop agreeing under an expanded parent
- locked rows: reorder allowed, re-parent refused
- the two-state keep model: multiselection cleared by a plain click, keep set
  surviving both that and navigation, released only by the parent's control
- trailing-slash create; `+` absent at rest and present on hover/focus
- tray: countdown commits, cancel restores subtree and repo contents

Backend, `TestRegression_*` in `api/tests` per house rule, black-box:

- folder CRUD and nesting under a protected branch
- reparent refused when it would split a fork chain
- order persisted and dense after every move
- repo moved between projects keeps its workspaces

## Sequencing

1. **Folders + ordering schema and API.** Everything rides on it.
2. **Tree render + drag rules.** Largest frontend piece and where the tests are.
3. **Interaction polish**: hover `+`, glyph swap, collapse animation,
   counts-as-subtitle. Each is small and independently revertable.
4. **Keep set last, or behind a flag.** It is the most novel thing here and the
   least exercised by anyone but its author.

## Known gaps

Carried deliberately, not overlooked:

- No keyboard navigation. The rows are already `role="treeitem"` in a
  `role="tree"`, which promises arrow keys, type-to-jump and Enter; the promise
  is currently unmet and the refactor should close it.
- No empty, loading or error states in the prototype — the real sidebar has
  placeholder workspaces mid-provisioning, spinners, and a daemon that drops.
- Nothing below ~280px, no touch.
- Trailing-slash folders, ⌘-click keep and drag-to-editor removal have no
  affordance at rest. Accepted; worth watching once it is in use.
- A workspace with children and a folder look alike but behave differently on
  drop (re-parent vs move). Watch for confusion.
