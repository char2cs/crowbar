# `repo-section` (P3.61)

`web/src/components/layout/repo-section.tsx` →
`crates/crowbar-ui/src/components/repo_section.rs`,
`crates/crowbar-app/src/surfaces/repo_section.rs`,
`crates/crowbar-app/src/row_layout/repo_section.rs`.

**No live reference.** This item does not run the oracle or capture a
snapshot — per the item brief's hard constraints. Every number below is read
off the app's own compiled Tailwind (`native/MAPPING.md`'s method) or
transferred from an existing measurement, not off a live capture.

Third of `native/mapping/layout-denominator.md` §8's Cluster 8.

## 0. What this file is, and what it is not

`repo-section.tsx` is `RepoSection()`: one repo in the workspaces sidebar —
its header row (which *is* the repo-home default workspace's own row) plus,
when expanded, the inline root-level create input, any optimistic
pending-create rows, and the workspace tree itself.

Confirmed **LIVE** by `native/mapping/layout-denominator.md` §2/§4, not by
`liveness-audit.md`.

## 1. Values

| React / Tailwind | Compiles to | gpui / `crowbar-ui` |
|---|---|---|
| `ROW_BASE` + `mx-1.5 my-0.5` | — | `row_base::base` + `.mx(MARGIN_X).my(MARGIN_Y)` — see §2 |
| `ROW_ACTIVE`/`ROW_INACTIVE` | — | `row_base::active`/`inactive` |
| `mb-1` (outer wrapper) | 4px | literal `px(4.0)` |
| `paddingLeft: 14` (root-level create/pending rows) | 14px | `ROOT_PADDING_LEFT` |

## 2. `mx-1.5`/`my-0.5` ARE modelled — a section always sits beside siblings

`workspace-tree.tsx` renders one `RepoSection` per repo, stacked — the same
"sits beside siblings" shape `workspace-tree-item.rs`'s own module docs
argue for its own row. [`RepoSection::header`] applies `row_base::MARGIN_X`/
`MARGIN_Y`, wrapped in its own `flex flex-col` single-item context so the
row stretches to fill it net of that margin — no `.w_full()`, the same
`.w_full()`-next-to-`.mx()` trap `workspace_tree_item.rs`'s own row guards,
guarded here too (`row_layout::repo_section::the_header_is_inset_by_
margin_x_on_both_edges`, run).

## 3. `mode: RowMode` — a folded field, not two bools

`isRenaming`/`isCreatingChild` (repo-rename vs. root-level create-child)
fold into [`row_base::RowMode`], the identical division `workspace-tree-
item.rs`'s own module docs describe — a row cannot show both an inline
rename field and an inline create-child field at once. Keeps [`RepoSection`]
at three `bool`s (`is_active`, `is_collapsed`, `has_default_workspace`),
exactly clippy's `struct_excessive_bools` limit.

## 4. Composes `repo_icon_popover::Trigger::render` directly

Single instance per section (there is exactly one avatar per repo row), so
this is collision-free the same way `project_home_row.rs`'s single
composition of `WorkspaceBranchIcon::render` is — `repo_icon_popover.rs`'s
own module docs already established that its trigger's two rest states are
real, tested geometry with no `oracleSurfaceScope` entry needed, because
what it composes is exactly what it paints. Reusing it here rather than
hand-rolling a second copy of the avatar's three-way picture (image/emoji/
letter) is that restraint, applied at a second call site.

## 5. `RepoImportDialog` is not composed

`repo-section.tsx` renders `<RepoImportDialog ... open={importOpen} .../>`
unconditionally, starting closed — the same `AddRepositoryModal`/
`ImportProjectModal` posture `project_home_row.rs`'s and `project_switcher_
panel.rs`'s own module docs already take. This section's own painted
picture never depends on whether the dialog is open, so nothing is omitted
by leaving it out.

## 6. The label slot — the React source's own second wrapper is not modelled

`repo-section.tsx` wraps the name in a second, `flex-1` span (to leave room
for a hover-only "- default" hint beside it, itself not modelled — hover-
only content has no runtime seam anywhere in this port). With nothing
rendered beside it, `row_base::label_container`'s own `min-w-0 flex-1` is
exactly that wrapper's own box, and stacking a second identical one around
it would record the same bounds twice — `row_base.rs`'s own
`label_container` doc comment, applied here rather than re-derived.

## 7. Two foreign, not-yet-ported children, same treatment as `workspace_tree_item.rs`

The rename slot (`workspace-inline-input.tsx`) has no wrapper of its own in
the React source and is left unanchored. The root-level create-input row's
own chrome (`ID_CREATE_INPUT`) *is* this component's own markup, so it is
anchored; `WorkspaceInlineInput` inside it is not. See
`workspace_tree_item.rs`'s own module docs (§6) for the full argument,
restated once rather than twice.

## 8. Anchoring

`repo-section.tsx` carried no `data-oracle-id` before this item. Six are
added:

* `repo-section` — the `role="treeitem"` header div, this surface's own
  root. Repeats once per repo in a wider capture (`workspace-tree`'s own
  scope excludes the family — see `workspace-tree.md` §8).
* `repo-section-label` — the truncating repo-name span. Renaming branch
  only.
* `repo-section-import` — the "Import branches" trailing action.
* `repo-section-add-child` — the add-child trailing action.
  `repo.defaultWorkspaceId` only.
* `repo-section-collapse` — the collapse/expand trailing action.
* `repo-section-create-input` — the root-level inline create row's own
  chrome. `isCreatingChild` only.

The composed `repo-icon-popover-trigger` (and, one level deeper on the
image case, `repo-avatar`) reuses `repo-icon-popover.tsx`'s own existing
id, unmodified by this item.

### The scope-entry decision, argued in full

**`web/src/lib/oracle/extract.ts` declares `repo-section`.**
`repo-icon-popover-trigger` is real and painted by this composition (§4),
not foreign, so it is declared rather than excluded — unlike `workspace-
tree-item.tsx`'s own icon, this surface's own avatar trigger does **not**
recur within one `repo-section` capture (there is exactly one avatar per
section, regardless of how many workspace rows follow it), so declaring it
carries none of `workspace-tree-item`'s own leaf-only caveat.

Not declared: the recursive `workspace-tree-item`/`pending-create-row`
family — `select-item`'s reasoning, one level up (their count is this
repo's own workspace-list length, a cell property). `repo-section-label`
is declared (present on every capture that is not mid-rename, the
overwhelmingly common and only currently-reachable shape); `repo-section-
add-child` is cell-conditional on `repo.defaultWorkspaceId` and left
undeclared, `popover-title`'s own reason.

## 9. Declarations

`CONTENT_SIZED = []`. `LINE_SIZED = [repo-section-label]` — a blockified
flex item with no explicit height of its own.

## 10. The state axis

| flag | here |
|---|---|
| `selected` | **real.** `activeWorkspaceId === repo.defaultWorkspaceId` selects `row_base::active` over `row_base::inactive`, the same reading every other row-shaped surface in this port gives it. |
| `empty`, `loading`, `error`, `hover`, `focus` | **unmodelled.** `empty` has no trailing-edge concept on this row. `hover`/`focus` are colour-only. `isRepoDragOver` (`ring-1 ring-ring`) is sourced from `workspace-tree-context.tsx`'s Phase 5 drag protocol — out of this item's scope, `workspace_tree_item.rs`'s own dragging/moving/drop-target reasoning, restated once for this surface. |

`Params::no_state_axis()` returns `false` — one real flag (`selected`).

## 11. `row_layout` coverage

* the default cell (one expanded root, a default workspace) carries the
  header's full anchor set plus one nested `workspace-tree-item`
* `--no-default-workspace` drops the add-child action
* `--collapsed` hides the workspace list entirely — no `workspace-tree-item`
  reaches the capture — while the header stays
* `--roots 3` nests three `workspace-tree-item` roots
* the header row is inset by `MARGIN_X` on both edges — the `.w_full()`
  trap, guarded by a run mutation
* the header keeps its authored `row_base::HEIGHT` whether or not selected
* the label's own line box is `13 × LINE_HEIGHT_RELATIVE` (19.5px) —
  closing the same gap `workspace-tree-item.md` §11 describes, for this
  surface's own label

## 12. Reachability

`workspace-tree.tsx` → `sidebar-carousel.tsx` → `ide-shell.tsx` →
`routes/_shell.tsx`. One instance per repo, always mounted above the tree
it wraps.

---

## VERDICT: FAIL — 5 deltas over 5 anchors (2026-08-03, my own run)

Final drive: `--surface repo-section --width 344 --viewport-width 1684 --theme
dark --content normal --name demo --roots 0`.

**My first drive produced 12 deltas; seven were my own.** Recorded because both
mistakes are ones this port keeps repeating:

- **`--width` is the CONTAINER, not the row.** I passed `--width 332` (the
  reference root's own `bounds.w`) and got `repo-section.bounds.w: 320` —
  because the port applies `row_base::MARGIN_X` (`mx-1.5`, 6px a side) inside
  the width it is given. The reference row is 332 *because its container is
  344*. Passing 344 closed that delta and the two action-button `x` deltas
  that were only ever the same 12px.
- **`--roots 1` renders a child row the reference's scope excludes**, so the
  port emitted four anchors (`workspace-tree-item*`, `workspace-branch-icon`)
  the reference never carries. `--roots 0` is the cell that matches this
  surface's declared anchor set.

### The four real findings

1. **`repo-section-add-child` is emitted by the port and *dropped* from the
   reference — and the reference is the one that is wrong.** The scope entry at
   `extract.ts:1588` declares exactly five anchors and omits it. I checked the
   live DOM rather than assuming: the element is a **24×24, `display: flex`,
   `visibility: visible`, `opacity: 1`** box inside every `repo-section`. It is
   this surface's own chrome, not a repeated child-row family, so the
   `select-item` precedent the neighbouring entries cite does not cover it.
   **Left as-is it is permanent**: the port emits a correct anchor and the
   contract refuses to see it, for ever.
2. **`repo-section-label.bounds.w` — 202.0 against 31.2.** The reference's
   label width equals its `text_width` **exactly** (31.2), so it is
   content-sized; the port stretches it to fill. Note this is the *opposite* of
   `project-home-row`, whose label legitimately stretches (232 wide against a
   109.2 `text_width`) because it carries `min-w-0 flex-1 truncate`. Same
   `row_base` chrome, different sizing — do not transfer one to the other.
3. **`repo-icon-popover-trigger.radius` — 0.0 against 8.0.** The port does not
   round the trigger.
4. **`repo-section-import` and `repo-section-collapse` `border.w` — 1.0 against
   0.0.** The port paints a 1px border on both action buttons where React has
   none.

### What passed, and is worth noting

`--name demo` matched the live label exactly — **no `text` delta**. This
surface already has the fixture flag that `project-switcher-panel`,
`repo-avatar` and `repo-icon-popover` all lack, and it is the reason this
verdict could be taken against a real repo at all.

`repo-section-label.font.line_height` is **19.5** on both sides.
