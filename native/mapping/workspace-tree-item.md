# `workspace-tree-item` (P3.61)

`web/src/components/layout/workspace-tree-item.tsx` →
`crates/crowbar-ui/src/components/workspace_tree_item.rs`,
`crates/crowbar-app/src/surfaces/workspace_tree_item.rs`,
`crates/crowbar-app/src/row_layout/workspace_tree_item.rs`.

**No live reference.** This item does not run the oracle or capture a
snapshot — per the item brief's hard constraints. Every number below is read
off the app's own compiled Tailwind (`native/MAPPING.md`'s method) or
transferred from an existing measurement, not off a live capture.

Second of `native/mapping/layout-denominator.md` §8's Cluster 8.

## 0. What this file is, and what it is not

`workspace-tree-item.tsx` is `WorkspaceTreeItem()`: one row of the
workspace tree — a status icon, the branch name (or an inline rename
field), an optional change-count cluster, a trailing expand/collapse or
add-child control, and, recursively, its own children plus any in-flight
creates beneath it. It is the closest thing in `components/layout` to the
Phase 1 gate's `tree-row.tsx`, generalized to a workspace, per
`native/mapping/layout-denominator.md` §6's own reading.

Confirmed **LIVE** by `native/mapping/layout-denominator.md` §2/§4, not by
`liveness-audit.md`.

## 1. Values

| React / Tailwind | Compiles to | gpui / `crowbar-ui` |
|---|---|---|
| `ROW_BASE` + `mx-1.5 my-0.5` | — | `row_base::base` + `.mx(MARGIN_X).my(MARGIN_Y)` — see §2 |
| `ROW_ACTIVE`/`ROW_INACTIVE` | — | `row_base::active`/`inactive` |
| `(depth + 1) * 14` (row's own `paddingLeft`) | — | `WorkspaceTreeItem::row_padding_left` |
| `(depth + 2) * 14` (children/pending `paddingLeft`) | — | `WorkspaceTreeItem::nested_padding_left` |
| `gap-1` (change-count cluster) | 4px | `CHANGES_GAP` |
| `mx-1.5 mb-0.5 rounded-b-lg px-2.5 pb-2 pt-0.5` (placeholder details) | 6/2/10/8/2px | hand-built in `placeholder_details` |
| `size-4` (`<Plus>` in the "+ New" row) | 16px | literal `px(16.0)` — see §3 |

## 2. `mx-1.5`/`my-0.5` ARE modelled — a row always sits beside siblings

Every real workspace-tree-item instance is a member of a list: siblings
under the same `repo-section`'s root, or recursive children under a parent
row. `row_base.rs`'s own module docs put this in the "applies `MARGIN_X`/
`MARGIN_Y`" bucket (`project_switcher_panel.rs`'s own shape), never the
"standalone root" one (`project_home_row.rs`'s). [`WorkspaceTreeItem::row`]
applies both, and its own padding wrapper is `flex flex-col` so the row
stretches to fill it net of that margin — no `.w_full()` — the exact bug
the item brief calls out by name, and the one `row_layout::
workspace_tree_item::the_row_is_inset_by_margin_x_on_both_edges` guards
with a run mutation (§8).

## 3. `icon: WorkspaceBranchIcon` and `mode: RowMode` — folded fields, not
bools

Two clippy-motivated foldings, both real divisions rather than dodges:

* **`icon`** bundles `status`/`working`/`isPlaceholder` — exactly
  `WorkspaceBranchIcon`'s own three-way input, reused as a single field
  rather than three loose ones this component only ever forwards unchanged
  to [`WorkspaceBranchIcon::render`].
* **`mode`** bundles `isRenaming`/`isCreatingChild` into
  [`row_base::RowMode`] — a row cannot show both an inline rename field and
  an inline create-child field at once (they occupy the same slot), and
  `repo-section.tsx`'s own row needs the identical fold, so it lives on
  `row_base` rather than being invented twice.

Both keep [`WorkspaceTreeItem`] at two `bool`s (`is_active`, `expanded`),
under clippy's `struct_excessive_bools` without hiding a real distinction —
`button.rs`'s own `Props`/`Interaction` split, restated here.

## 4. `hasChildren`/`isCreatingChild`/`showPlaceholderDetails` are derived, not stored

* `WorkspaceTreeItem::has_children()` is `!self.children.is_empty()` —
  `hasChildren` is itself `children.length > 0` in the React source, so
  storing it as an independent field would let a fixture disagree with its
  own children list.
* `WorkspaceTreeItem::show_placeholder_details()` is `self.icon.is_placeholder
  && self.is_active`.
* `is_renaming`/`is_creating_child` read through `self.mode.is_renaming()`/
  `is_creating_child()` (§3).

## 5. The status icon composition is oracle-safe only for a leaf cell

Unlike [`super::pending_create_row`] (a genuine leaf), this component
recurses, so a cell with children present paints the shared literal
`workspace-branch-icon` id more than once. Sound for `cargo test`'s own
`row_layout` harness (`AnchorRegistry::record` keeps differing records
rather than refusing them; `Snapshot::build`'s v1.8 refusal is never
reached because `row_layout` tests never call `.snapshot()`), not sound for
a live oracle capture with children present — recorded in full in
`web/src/lib/oracle/extract.ts`'s own `workspace-tree-item` entry (§6).

## 6. Two foreign, not-yet-ported children, two different treatments

`placeholder-row-actions.tsx` (Retry/Detach…) and `workspace-inline-
input.tsx` (the rename/create-child text field) are each a **separate,
not-yet-landed Tier B target** — neither is this item's to port.

* **The placeholder-details wrapper is anchored.** Its `mx-1.5 mb-0.5
  rounded-b-lg … bg-background …` box is this component's own real chrome
  (background continuation, padding) regardless of what renders inside it,
  so [`ID_PLACEHOLDER_DETAILS`] is a real anchor and `PlaceholderRow-
  Actions`'s own content inside it is left empty.
* **The rename label and the create-child input are not.** Neither has a
  wrapper of its own in the React source — `WorkspaceInlineInput` sits
  directly in the row's children list — so this port paints an empty,
  unanchored box in that slot rather than inventing a wrapper (and an id)
  around content it does not own. The **rows** that would host the
  create-child input (`ID_CREATE_INPUT`) and the static "+ New" button
  (`ID_NEW_BUTTON`) *are* anchored — that chrome is this component's own —
  but the input itself, inside the former, is not.

Neither foreign component carries any `data-oracle-id` of its own (checked
against both sources), so no `oracleSurfaceScope` exclusion is needed for
either — there is nothing for a live capture to leak that this port's own
tree does not already omit by construction.

## 7. Anchoring

`workspace-tree-item.tsx` carried no `data-oracle-id` before this item.
Nine are added:

* `workspace-tree-item` — the `role="treeitem"` div, this surface's own
  root. Recurses: every nested child row carries the same literal id — see
  §5 and §8.
* `workspace-tree-item-label` — the truncating branch-name span. Renaming
  branch only (§6).
* `workspace-tree-item-added`/`-deleted` — the `+N`/`-N` change-count
  spans. Active, not renaming, not locked, positive count only.
* `workspace-tree-item-expand` — the chevron. `hasChildren` only, mutually
  exclusive with `-add-child`.
* `workspace-tree-item-add-child` — the leaf row's add-child action.
  `!hasChildren && !isCreatingChild` only.
* `workspace-tree-item-placeholder-details` — see §6.
* `workspace-tree-item-create-input` — the inline create-child row's own
  chrome. `isCreatingChild` only, mutually exclusive with `-new-button`.
* `workspace-tree-item-new-button` — the static "+ New" button.

The composed `workspace-branch-icon` (and, one level deeper on a
working/placeholder cell, `flicker-spinner`) reuses `workspace-branch-
icon.tsx`'s own existing id, unmodified by this item.

### The scope-entry decision, argued in full

**`web/src/lib/oracle/extract.ts` declares `workspace-tree-item`, and the
declared scope is sound for a leaf cell only.** The component recurses (it
renders itself for `children`), and both the root and the composed icon are
the same literal id at every depth — `select-item`'s own "count is a cell
property" reasoning means neither the recursive family nor `pending-
create-row` (its own registered surface) is declared. Unlike `repo-
section`/`workspace-tree` below, the repeated id here is not confined to
an *excluded* child: it is this surface's own root and its own icon, so a
capture with any child present carries two elements under one declared id
and `oracleSelectDeclaredAnchors`' "two elements carry one declared id"
rule refuses it outright, the identical shape `search --replace`'s two
`Input`s hit. The one reachable reference this scope can validate is a
childless row; a deeper cell is a future worker's problem (its own
registered surface, `search-replace-row`'s own precedent), not something
declared here.

The row's own conditional slots (`-added`/`-deleted`, `-expand`,
`-placeholder-details`, `-create-input`) are left undeclared too,
`dialog-header`'s own reason: each is real on some reachable cell and
absent on others.

## 8. Declarations

`CONTENT_SIZED = []`. `LINE_SIZED = [workspace-tree-item-label,
workspace-tree-item-added, workspace-tree-item-deleted]` — three
blockified flex items with no explicit height of their own,
`project_home_row::LINE_SIZED`'s own shape.

## 9. The state axis

| flag | here |
|---|---|
| `selected` | **real.** `isActive` selects `row_base::active` over `row_base::inactive`, the same reading every other row-shaped surface in this port gives it. |
| `empty`, `loading`, `error`, `hover`, `focus` | **unmodelled.** `empty` has no `git-status-row`-shaped "nothing on the trailing edge" concept here. `hover`/`focus` are colour-only ([`row_base`]'s own module docs). Dragging (`opacity-40`), moving (`opacity-50`) and being a drop target (`ring-1`) are real props on the React source but are sourced entirely from `workspace-tree-context.tsx`'s pointer-drag protocol, which `native/mapping/layout-denominator.md` §6 classifies **Phase 5 (interaction), not Tier B, with no geometry of its own** — out of this item's scope, so none of the three has a field here. |

`Params::no_state_axis()` returns `false` — one real flag (`selected`).

## 10. `row_layout` coverage

* the default (leaf) cell carries the root, icon, label and add-child
  action — never the expand chevron
* `--children 1` swaps the leaf's add-child for the expand chevron and
  recurses: a second `workspace-tree-item` root is recorded
* a child sits exactly one indent level (`14px`) to the right of its parent
  — the mutation record shows the first attempt (mutating `depth_padding`'s
  own multiplier) did **not** bite, because an additive shift cancels in a
  delta between two calls at consecutive levels; the real assertion is
  carried by the surface's own `depth + 1` increment on generated children
* `--pending 1` nests a `pending-create-row` at `(depth + 2) * 14`
* the row is inset by `MARGIN_X` on both edges, net of its own indentation
  — the `.w_full()` trap, guarded by a run mutation
* the root keeps its authored `row_base::HEIGHT` whether or not selected
* the icon sits flush against the row's own leading edge; the label
  follows by `GAP`
* the label's own line box is `13 × LINE_HEIGHT_RELATIVE` (19.5px), not the
  row's authored height — closing the gap a live parity run found in
  `project_home_row.rs`'s identical composition (§11)

## 11. `row_base::LINE_HEIGHT_RELATIVE` — inherited, not re-derived

A live parity run (after this item's own first pass landed) found
`row_base::LINE_HEIGHT_RELATIVE` was `18.0 / 13.0` where Tailwind's own
inherited preflight `line-height: 1.5` (unitless, so it recomputes against
each descendant's own font-size) gives `19.5px` at `13px`, not `18px` —
confirmed against the live DOM. That fix landed in `row_base.rs` itself
(P3.60), so every consumer of `row_base::label_container` — this file
included — inherited the correction automatically; nothing in this file's
own arithmetic needed to change. What this file's own `row_layout` module
*did* need, and did not have before the parity run: an assertion on the
label's own line-box height, not only the row's authored height — added in
§10, run against the wrong ratio to confirm it would have caught the
original defect.

## 12. Reachability

`repo-section.tsx` → `workspace-tree.tsx` → `sidebar-carousel.tsx` →
`ide-shell.tsx` → `routes/_shell.tsx`. Recursively self-reachable for every
workspace with children.

---

## VERDICT: FAIL — 1 delta over 3 anchors, and it is the **contract's**, not the port's (2026-08-03)

Drive: `--surface workspace-tree-item --width 344 --viewport-width 1684 --theme
dark --content normal --branch main --row-depth 0`.

```
workspace-tree-item-add-child: anchor present in the native snapshot but not in the reference

oracle: FAIL — 1 delta over 3 anchors compared (1 anchor presence)
```

**Every geometry, colour and typography field matches exactly.** Root, branch
icon and label — position, size, radius, border, font size, weight, family and
`line_height` (19.5) — all inside tolerance on the first correctly-driven run.
This surface would be **PASS 0/3** if the reference carried the anchor the port
emits.

### The delta is an under-declared scope entry

`extract.ts`'s entry declares three anchors:

```ts
'workspace-tree-item': {
  root: 'workspace-tree-item',
  anchors: ['workspace-tree-item', 'workspace-branch-icon', 'workspace-tree-item-label'],
},
```

`workspace-tree-item-add-child` is missing. I checked the live DOM rather than
reasoning about it: **two instances, each 24×24, `display: flex`, `visibility:
visible`, `opacity: 1`.** It is this row's own chrome, not a repeated child-row
family, so the `select-item` precedent the neighbouring entries cite does not
reach it.

**`repo-section` has the identical omission** (`repo-section-add-child`, also a
live 24×24 box). Two surfaces, same missing anchor kind, same commit — so this
is one systematic slip in P3.61's scope entries, not two coincidences.

### The drive, and the width trap's third spelling

The first run reported `bounds.w` 304 against 318 on both the root and the
label. `--width` is the **container**: at `--row-depth 0` the port insets the
row by 26px total, so a 318px row needs `--width 344` — the sidebar panel's own
width. Passing the reference's root `bounds.w` (318) is exactly wrong. Both
deltas closed at 344 and nothing else moved.

### Re-verified after P3.65 — the contract fix exposed a real defect

With `workspace-tree-item-add-child` declared, the presence delta is gone and
the newly-compared anchor matches on **position, size and radius exactly**
(`x 287, y 6, w 24, h 24, radius 10`). One delta remains:

```
workspace-tree-item-add-child.border.w: 1.0, expected 0.0 (exact)

oracle: FAIL — 1 delta over 4 anchors compared (1 geometry)
```

**This is a genuine port defect that the under-declaration was hiding.** The
anchor was not being compared at all, so a wrong border width could not fail.
Fixing the contract did not just clear a false delta — it surfaced a true one,
which is the argument for declaring every anchor that renders rather than the
ones a surface happens to care about.

It is also **the same defect already recorded on `repo-section`**
(`repo-section-import` and `repo-section-collapse`, both `border.w` 1.0 against
0.0). Three anchors, two surfaces, one cause: the port paints a 1px border on
row action buttons where React paints none. Worth fixing once, at whatever
shared button path these three go through, rather than per surface.

---

## FIXED (2026-08-04, follow-up item)

The shared cause was confirmed and fixed once, at `row_base::sub_action_box`
(`crates/crowbar-ui/src/components/row_base.rs`), the single function backing
every `ROW_SUB_ACTION` box in this port: this surface's own
`workspace-tree-item-add-child`, plus `repo-section`'s `repo-section-import`,
`repo-section-add-child` and `repo-section-collapse`. The function used to
chain `.border(button::BORDER_WIDTH).border_color(Color::TRANSPARENT)` onto
every box it returned — copied, by analogy, from `ROW_BASE`'s own shell
(`row_base::base`) and from `button.tsx`'s "every anchored button reports
`border.w: 1`" finding, neither of which applies here.
`ROW_SUB_ACTION`'s own class list (`workspace-row-base.ts`) is `inline-flex
shrink-0 cursor-pointer rounded-lg p-1.5 text-muted-foreground hover:…
focus-visible:…` — no `border` utility anywhere in it, so the live DOM paints
no border at all on any of these four buttons. The two lines were removed.

**Regression guard, run against a real mutation:** restoring those two lines
and running `cargo test -p crowbar-app --bin crowbar-app
row_layout::workspace_tree_item::the_add_child_action_paints_no_border`
failed as predicted (`expected 0px, got 1px`,
`crates/crowbar-app/src/row_layout/workspace_tree_item.rs`); reverted after
confirming. `repo_section.rs`'s own sibling test
(`the_trailing_actions_paint_no_border`) guards the other three call sites.
