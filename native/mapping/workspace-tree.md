# `workspace-tree` (P3.61)

`web/src/components/layout/workspace-tree.tsx` →
`crates/crowbar-ui/src/components/workspace_tree.rs`,
`crates/crowbar-app/src/surfaces/workspace_tree.rs`,
`crates/crowbar-app/src/row_layout/workspace_tree.rs`.

**No live reference.** This item does not run the oracle or capture a
snapshot — per the item brief's hard constraints. Every number below is read
off the app's own compiled Tailwind (`native/MAPPING.md`'s method) or
transferred from an existing measurement, not off a live capture.

Last of `native/mapping/layout-denominator.md` §8's Cluster 8, and the
chain's own root.

## 0. What this file is, and what it is not

`workspace-tree.tsx` is `WorkspaceTree()`: the sidebar's own workspace
tree — the project-home row, a scrolling column of repo sections, each
holding its own workspace rows, and (in the real source) a trash-drop
footer this item omits (§4).

Confirmed **LIVE** by `native/mapping/layout-denominator.md` §2/§4, not by
`liveness-audit.md`.

## 1. This is the one component in the chain that IS `anchors.root`

`pending-create-row`, `workspace-tree-item` and `repo-section` are all
always composed (by a parent, or by themselves recursively), so all three
use `AnchorSink::boxed` for their own root. Nothing composes
`WorkspaceTree` — `workspace-tree.tsx`'s sole importer is `sidebar-
carousel.tsx`, which, per `native/mapping/layout-denominator.md` §1,
renders the four panels it wraps **empty**, this port included — so this
file is the chain's genuine top: [`WorkspaceTree::render`] calls
`AnchorSink::root`, `project_home_row.rs`'s own shape.

### A latent bug this composition found, and fixed

`ProjectHomeRow::render` called `AnchorSink::root` too, before this item.
Nothing had ever composed `ProjectHomeRow` inside a parent surface before
Cluster 8 landed (its sole importer was always `workspace-tree.tsx`, per
its own module docs), so the bug was never triggered. `AnchorSink::root`
clears every anchor recorded so far on entry — correct for a genuinely
top-level surface, silently wrong for one nested inside another: composing
it inside `WorkspaceTree::render` reproducibly dropped `workspace-tree`'s
own root anchor from every capture, found by this item's own `row_layout`
tests failing with the outer root missing while everything nested beneath
`project-home-row`'s own composition still appeared. Fixed in
`project_home_row.rs` (`.boxed`, not `.root`) — see that file's own module
docs for the full account.

## 2. `WorkspaceTreeFooter` is omitted, not stubbed

`workspace-tree.tsx` renders `<WorkspaceTreeFooter />` as a sibling of the
scroll area, always mounted ("always rendered so the `ScrollArea` doesn't
resize on drag start/end", the source's own comment). `native/mapping/
layout-denominator.md` §6 classifies it **Phase 5 (interaction — drag), not
Tier B**, and flags the resulting composition decision as unmade — this
item makes it: **omit**, not stub.

Its own resting picture (no drag in progress) is `max-h-0 overflow-hidden`
— zero height, by its own class list, regardless of whether the element
exists at all. Its data feed (`draggingWs`/`hoverTargetId` off
`useWorkspaceTreeDrag()`) is `workspace-tree-context.tsx`'s Phase 5
pointer-drag protocol, itself out of Tier B scope and not composed by
anything in this chain. So an omitted footer and a present-but-`max-h-0`
one paint the identical picture in every state this port's own resting
captures reach — the same `AddRepositoryModal`-shaped argument `project_
home_row.rs`'s own module docs make, reapplied to a sibling rather than a
portal. A worker who later ports `workspace-tree-footer.tsx` for its own
sake (Phase 5) is free to compose it here; this item does not block that.

## 3. `ScrollArea::render` is not called; its two ids are reused by hand

`scroll_area::ScrollArea::render` paints a **synthetic, empty** body at a
caller-fixed pixel extent — its own module docs: "unanchored on purpose…
it stands in for whatever a call site rendered." This composition needs
the opposite: the real repo-section list, sized by `flex-1` rather than a
fixed number. So this file does not call `ScrollArea::render` — the same
"does not call `X::render`" posture `project_home_row.rs`/`context_pill.rs`
already take for `Button::render`, generalized here to a primitive whose
own API has no children slot at all — and instead hand-builds the
equivalent `overflow-hidden` viewport shape, reusing `scroll_area::ID_ROOT`/
`ID_VIEWPORT` directly: the real `<ScrollArea>` in `workspace-tree.tsx`
carries exactly those two ids on its root and viewport regardless of what
it wraps, so reusing the constants keeps this port's anchor set matching
the real DOM rather than inventing a third name for the same two boxes.

## 4. The `error` early return is not modelled

`wsListData.status === 'error' && repos.length === 0` swaps the whole tree
for `<InlineError>` — `StateFlag::Error` is mandatorily unmodelled on every
surface in this port (`crowbar-app/src/surface.rs`'s own invariant), so
`WorkspaceTree` has no field for it, and the React diff does not tag that
branch's own root with `data-oracle-id="workspace-tree"` (it renders a
structurally different tree, and adding the id would misleadingly imply
this is a comparable resting state of the surface).

## 5. Anchoring

`workspace-tree.tsx` carried no `data-oracle-id` before this item. One is
added:

* `workspace-tree` — the outer `flex flex-1 flex-col overflow-hidden` div,
  this surface's own root (the main, non-error return only — see §4).

Everything else reachable under it is composed content already anchored by
its own port: `project-home-row`'s own five ids (unconditional),
`scroll-area-root`/`scroll-area-viewport` (§3), and, per repo,
`repo-section`'s own family (excluded from this surface's own declared
scope — see below).

### The scope-entry decision, argued in full

**`web/src/lib/oracle/extract.ts` declares `workspace-tree`: the outer
scaffold only** — the root, `project-home-row`'s own five ids (already
declared under their own names, reused rather than repeated here —
`command`'s own `scroll-area-*` reuse, one call further), and the
hand-built `scroll-area-root`/`scroll-area-viewport` pair. `repo-section`
is excluded for `select-item`'s reason one level up: the repo count is a
property of the workspace list, not of this surface. Whatever a
`repo-section` capture nests (in turn excluding its own `workspace-tree-
item`/`pending-create-row` families) is verified through that surface's
own root, never through this one — `resizable`'s own precedent for a
container whose repeated content is somebody else's surface.

## 6. Declarations

`WorkspaceTree` paints no text of its own — every text run reachable under
its root belongs to a composed child (`project-home-row`'s label,
`repo-section`'s, `workspace-tree-item`'s), each already declared on its
own surface. No `CONTENT_SIZED`/`LINE_SIZED` array on this file.

## 7. The state axis

| flag | here |
|---|---|
| `loading`, `error`, `hover`, `focus`, `selected`, `empty` | **unmodelled.** This surface is a container, not a row — it has no selection/hover/focus picture of its own, and its own `empty` (zero repos) would be a real, drivable state (`--repos 0` already reaches it structurally) but is left undeclared as a §8.3 flag: nothing in `native/mapping/layout-denominator.md` flags a live "zero repos" reference to validate a field against, and inventing one without it is the trap `ANCHORS.md` §6 warns against. `--repos <n>` still lets a future worker drive the count directly. |

`Params::no_state_axis()` returns `true` — none of the six §8.3 flags has
a rule on this surface, `workspace_branch_icon.rs`'s own precedent for the
declaration.

## 8. `row_layout` coverage

* the default cell (one repo) carries the root, the full `project-home-row`
  family, the hand-built scroll-area pair, and one nested `repo-section`
* `--repos 3` nests three `repo-section` roots
* `--repos 0` still carries the scaffold — no `repo-section` at all,
  nothing else missing

No label-height assertion in this file's own `row_layout` module — see §6:
this surface paints no text run of its own for such an assertion to name.

## 9. Reachability

`sidebar-carousel.tsx` → `ide-shell.tsx` → `routes/_shell.tsx`. Always
mounted above the repo tree, per `native/mapping/layout-denominator.md`'s
own table.

---

## VERDICT: FAIL — 19 deltas over 8 anchors, **three causes** (2026-08-03)

Drive: `--surface workspace-tree --width 344 --viewport-width 1684 --theme dark
--content normal --repos 0`. (`--repos 1` adds ten anchor-presence deltas the
reference's own scope excludes — the repo count is declared a cell property one
level up, so `0` is the cell that matches this surface's declared anchor set.)

### A. The port does not apply `row_base::MARGIN_X`/`MARGIN_Y` to the row it composes — **13 of the deltas**

```
project-home-row.bounds.x:        0.0,  expected 6.0    (mx-1.5)
project-home-row.bounds.y:        0.0,  expected 2.0    (my-0.5)
project-home-row.bounds.w:      344.0,  expected 332.0  (344 − 2×6)
project-home-row-{icon,label}.x:  ±6                    (carried by the row)
project-home-row-{import,switch}.x: ±6
project-home-row-{icon,label,import,switch}.y: ±2 / 2.5
scroll-area-{root,viewport}.y:   36.0,  expected 40.0   (2 above + 2 below)
workspace-tree.bounds.h:        972.0,  expected 976.0
```

**`row_base`'s own module documentation predicted this exact consumer.** It
says `mx-1.5`/`my-0.5` are "not baked into [`base`]" because a row captured as
its own root has no anchor they could move, and that they are exported "as the
numbers a *list*-shaped consumer (`project-switcher-panel`, **eventually
`workspace-tree`**) applies to each row itself". `workspace-tree` is that
consumer, and it does not apply them. Every one of these thirteen is that single
omission observed through a different anchor.

### B. The composed home row renders **inactive**; the live one is active

```
project-home-row.bg:           #00000000, expected #1f1f1eff
project-home-row.border.color: #00000000, expected #1f1f1eff
```

`row_base::active` is `border-background bg-background` and that is what the
reference shows. The surface has **no axis** to say which row is active —
unlike `project-switcher-panel`, which has `--active-index`/`--no-active`. Not a
paint bug: a missing cell axis.

### C. No `--project-name`, so the label can never match — the **fourth** surface

```
project-home-row-label.text:       "home", expected "oracle-fixture"
project-home-row-label.text_width: 31.2,   expected 109.2
```

`project-home-row` has the flag; this surface composes that row and cannot pass
a name through. Same gap as `project-switcher-panel`, `repo-avatar` and
`repo-icon-popover` — four surfaces now, all permanently un-passable against any
real app state until each grows a flag.

### What passed

`project-home-row-label.font.line_height` is **19.5** on both sides, and the
`scroll-area-*` pair matches exactly in width. The row's *internal* layout is
correct — every delta above is about where the row sits, what state it is in, or
what string it holds, not how it is built.

---

## FIXED (2026-08-04, follow-up item)

All three causes are closed.

### Cause A — `row_base::MARGIN_X`/`MARGIN_Y` now applied at the one call site that needed them

`WorkspaceTree::render` used to call `self.project_home.render(theme,
anchors)` straight into its own column. `ProjectHomeRow::render`'s own
`.w_full()` (no margin) is correct for its *own* standalone
`--surface project-home-row` capture — see that file's own PASS verdict — but
wrong once composed beside `repo-section`'s own list, exactly the shape
`row_base.rs`'s own module docs predicted for this file by name. Fixed by a
new `WorkspaceTree::home_row` (`crates/crowbar-ui/src/components/
workspace_tree.rs`) that wraps the composed row in an outer, unanchored
`flex flex-col` container carrying `.mx(row_base::MARGIN_X)
.my(row_base::MARGIN_Y)` — without touching `ProjectHomeRow::render` itself,
avoiding the "no `.w_full()` next to `.mx()`" trap by putting the margin on a
*different* div than the one that already carries `.w_full()`. Closes all 13
of cause A's deltas, including the two that follow from it without further
code (`scroll-area-{root,viewport}.bounds.y` and `workspace-tree.bounds.h`,
both `+4px` from `MARGIN_Y * 2` now being in the flow).

### Cause B — `--home-active` added

`--surface workspace-tree` had no axis to say the composed home row is
active — `project-switcher-panel`'s `--active-index`/`--no-active`
precedent, simplified to a bare switch (`--home-active`) because this
surface composes exactly one such row. Added to `Params`
(`crates/crowbar-app/src/surfaces/workspace_tree.rs`); defaults to the row's
own idle/inactive fixture, so a bare `--surface workspace-tree` is
unchanged.

### Cause C — `--project-name` added

Threaded the same way `project-home-row`'s own surface and (per P3.64)
`project-switcher-panel`'s row 0 already do: `Params::tree` now overrides
the composed row's own `project_name` field when `--project-name` is passed,
leaving it on the `'home'` fixture fallback otherwise.

### Regressions guarded

`crates/crowbar-app/src/row_layout/workspace_tree.rs` gained four tests, each
run against a real mutation of the fix it guards and reverted after
confirming the real failure: `project_home_is_inset_by_row_base_margin_on_
both_axes` and `the_scroll_area_starts_below_the_home_rows_own_margin`
(cause A, one mutation, both failed);
`project_name_reaches_the_composed_home_rows_label` and
`home_active_paints_the_composed_row_active` (causes B/C, one mutation, both
failed). `crates/crowbar-app/src/surfaces/workspace_tree.rs` gained matching
unit-level coverage (`project_name_overrides_the_composed_home_row`,
`home_active_reaches_the_composed_row`).

---

## ⛔→✅ A later, shared-helper regression carried through this surface, and is now fixed (P3.81)

P3.66 (landed after the `FIXED` section above) removed a phantom border from
`row_base::sub_action_box` to close a real defect on `repo-section`'s and
`workspace-tree-item`'s own trailing actions — correct for those, and wrong
for `project-home-row`'s two actions, which carry a real transparent 1px
border in the live DOM (they render through the shared `<Button
variant="ghost">` primitive, not a raw `ROW_SUB_ACTION`-only element). This
surface composes `project-home-row` (`WorkspaceTree::home_row`), so the
regression carried straight through:

```
project-home-row-import.border.w:  0.0, expected 1.0
project-home-row-switch.border.w:  0.0, expected 1.0
```

**Fixed at the source** (`native/mapping/project-home-row.md`'s own
`REGRESSED`/`FIXED` sections) — `ProjectHomeRow::sub_action`
(`crates/crowbar-ui/src/components/project_home_row.rs`) now restores the
border at its own call site rather than on the shared
`row_base::sub_action_box`. No code change was needed in `workspace_tree.rs`
itself; the fix reaches this surface entirely through composition, the same
way the regression did.
