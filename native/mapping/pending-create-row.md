# `pending-create-row` (P3.61)

`web/src/components/layout/pending-create-row.tsx` →
`crates/crowbar-ui/src/components/pending_create_row.rs`,
`crates/crowbar-app/src/surfaces/pending_create_row.rs`,
`crates/crowbar-app/src/row_layout/pending_create_row.rs`.

**No live reference.** This item does not run the oracle or capture a
snapshot — per the item brief's hard constraints. Every number below is read
off the app's own compiled Tailwind (`native/MAPPING.md`'s method) or
transferred from an existing measurement, not off a live capture.

First of `native/mapping/layout-denominator.md` §8's Cluster 8 — the
strict dependency chain `pending-create-row` → `workspace-tree-item` →
`repo-section` → `workspace-tree`, landed together by one worker per the
brief's own instruction.

## 0. What this file is, and what it is not

`pending-create-row.tsx` is `PendingCreateRow()`: the optimistic row shown
while a workspace create is in flight — a spinner plus the branch name, or,
if the create failed, an inline `✕` mark, the branch name, a `"failed"`
caption and a dismiss button. Rendered at two call sites — the repo root
(`repo-section.tsx`, `paddingLeft: 14`) and nested under a parent workspace
(`workspace-tree-item.tsx`, `paddingLeft: (depth + 2) * 14`) — so every
create shows its loading animation regardless of where it forks from.

Confirmed **LIVE** by `native/mapping/layout-denominator.md` §2/§4, not by
`liveness-audit.md` (which covers only already-registered surfaces — a
citation error a previous worker made and retracted, per the item brief).

## 1. Values

| React / Tailwind | Compiles to | gpui / `crowbar-ui` |
|---|---|---|
| `h-9 px-1.5 gap-1.5 rounded-lg border` (`ROW_BASE`) | — | `row_base::base` |
| `border-transparent` | — | `Color::TRANSPARENT` |
| `opacity-60` | — | `ROW_OPACITY` |
| `mx-1.5 my-0.5` (`ROW_BASE`, applied here) | 6px / 2px | `row_base::MARGIN_X`/`MARGIN_Y` — see §2 |
| `pointer-events-none` | no hit-testing | not modelled — gpui has no such property to disable |
| `flex size-4 shrink-0 items-center justify-center` (icon slot) | 16px | `ICON_SIZE` |
| `text-xs` (error mark, `"failed"`, dismiss) | 12px | `TEXT_XS` |
| `ml-1` (dismiss) | 4px | `DISMISS_MARGIN_LEFT` |
| `text-[13px] text-muted-foreground` (label) | — | `row_base::label_container(theme.muted_foreground)` |
| `<WorkspaceAgentSpinner />` | — | composed `workspace_branch_icon::WorkspaceBranchIcon{ working: true }` — see §3 |

## 2. `mx-1.5`/`my-0.5` ARE modelled here, unlike `project-home-row`

`row_base.rs`'s own module docs record two shapes for its exported margin
constants: a row captured as its own standalone root (`project-home-row`)
does not apply them, because a captured element's own bounds exclude its
margin regardless; a row that sits beside siblings inside one list
(`project-switcher-panel`) does, because there the margin is the actual
spacing mechanism. `pending-create-row.tsx` is always the second shape — its
own `key={tempId}` in the React source is a hint that more than one
instance can coexist (several concurrent pending creates) — so
[`PendingCreateRow::render`] applies `row_base::MARGIN_X`/`MARGIN_Y`, the
same call `repo-section.tsx`'s and `workspace-tree-item.tsx`'s own rows
make (see those files' own mapping docs).

### The `.w_full()` trap, and how this file avoids it

Applying margin to a row that also needs to fill its container is exactly
the shape `project_switcher_panel.rs`'s own module docs warn about: an
explicit `width: 100%` does not shrink for its own margin the way flex
stretch does, overflowing the container by `2 * MARGIN_X`. This file's own
padding wrapper (`div().flex().flex_col().pl(padding_left)`) is therefore a
single-item `flex-col` context, and the row inside it carries no
`.w_full()` call at all — it stretches to fill the wrapper net of its own
margin, `project_switcher_panel.rs`'s own fixed pattern, applied here from
the start rather than rediscovered.

## 3. The spinner composes `workspace_branch_icon` directly; the error mark does not

`<WorkspaceAgentSpinner />` is, per `workspace-branch-icon.tsx`'s own module
docs, exactly `WorkspaceBranchIcon{ working: true }` under its own shared
`workspace-branch-icon` anchor. `PendingCreateRow::icon` reuses that
composition directly for the non-error branch — safe here specifically
because `PendingCreateRow` is a leaf with no internal repetition of its own
(unlike `workspace-tree-item.rs`, which recurses and therefore cannot reuse
the same composition without risking two `workspace-branch-icon` anchors in
one capture — see that file's own mapping doc).

The error branch's `✕` mark is a **different glyph** in the same slot, not
`WorkspaceBranchIcon`'s picture at all, so it is hand-painted under
[`ID_ICON`] — the row's own id — mutually exclusive with the composed one.

## 4. The outer padding wrapper carries no anchor

`<div style={{ paddingLeft }}>` is a plain positioning wrapper; the row
inside it (`ID_ROOT`) already reports its own absolute origin including
that offset, so a second anchor on the wrapper would not tell the differ
anything the row's own bounds do not already carry. Recorded as a
deliberate choice on both sides of the port — the React diff carries the
same reasoning as a comment.

## 5. Anchoring

`pending-create-row.tsx` carried no `data-oracle-id` before this item. Five
are added:

* `pending-create-row` — the `ROW_BASE` div, this surface's own root.
* `pending-create-row-icon` — the leading slot. Present on the **error**
  branch only (the spinner branch reuses `workspace-branch-icon` instead —
  see §3).
* `pending-create-row-label` — the truncating branch-name span, both
  branches, carrying `data-oracle-line-sized="true"` (v1.6 — see §6).
* `pending-create-row-status` — the `"failed"` caption. Error branch only.
* `pending-create-row-dismiss` — the dismiss button. Error branch only,
  content-sized (v1.5 — its own box is entirely its text run's advance
  width, no authored size at all).

### The scope-entry decision

**No `oracleSurfaceScope` entry.** `PendingCreateRow` is a genuine leaf: no
arbitrary call-site children, and its one composed nested anchor
(`workspace-branch-icon`, spinner branch) is a **single**, unconditional
instance per row — the same "composed and painted, not foreign and
unpainted, no entry needed" shape `project_home_row.rs`'s own identical
composition already resolved. Nothing here repeats within one
`pending-create-row` capture, so `select-item`'s "count is a cell property"
concern the three list-shaped surfaces upstream of this one (`workspace-
tree-item`, `repo-section`, `workspace-tree`) have to account for does not
arise here at all.

## 6. Declarations

`CONTENT_SIZED = []` on the two authored/flex-1 boxes;
[`ID_DISMISS`] is declared `content_sized` inline at its own call site
rather than listed in a `pub const` array (a single entry earns no array,
`project_home_row.rs`'s own "empty, for the same reason" shape one level
down). `LINE_SIZED = [pending-create-row-label, pending-create-row-status]`
— both blockified flex items in an `items-center` row with no explicit
height of their own, `project_home_row::LINE_SIZED`'s own shape.

## 7. The state axis

| flag | here |
|---|---|
| `loading`, `error`, `hover`, `focus`, `selected`, `empty` | **unmodelled.** This row has no selection/hover/focus rule of its own (`pointer-events-none`, in fact) and no `StateFlag::Empty`-shaped trailing-edge concept. `PendingCreateRow::error` is a **domain** field (`pending.error`) entirely orthogonal to the §8.3 `StateFlag::Error` — that flag stays mandatorily unmodelled here, as on every surface in this port (`crowbar-app/src/surface.rs`'s own invariant). |

`Params::no_state_axis()` returns `true` — none of the six §8.3 flags has a
rule on this surface, `workspace_branch_icon.rs`'s own precedent for the
declaration.

## 8. `row_layout` coverage

* the default (idle) cell carries the root, the composed spinner
  (`workspace-branch-icon`/nested `flicker-spinner`) and the label — never
  the three error-only anchors
* `--error` swaps the icon for the row's own mark and adds the status and
  dismiss anchors, and the composed spinner never appears
* the root's own absolute origin is the harness's own horizontal inset,
  plus `--padding-left`, plus the row's own `MARGIN_X` — read together
  rather than assumed, see the mutation record in the row_layout module
* the root keeps its authored `row_base::HEIGHT` whether idle or failed

## 9. Reachability

`repo-section.tsx`, `workspace-tree-item.tsx` → `workspace-tree.tsx` →
`sidebar-carousel.tsx` → `ide-shell.tsx` → `routes/_shell.tsx`. Always
reachable whenever a workspace create is in flight, from either of this
cluster's two other row components.
