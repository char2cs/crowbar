# `project-switcher-panel` (P3.60)

`web/src/components/layout/project-switcher-panel.tsx` →
`crates/crowbar-ui/src/components/project_switcher_panel.rs`,
`crates/crowbar-app/src/surfaces/project_switcher_panel.rs`,
`crates/crowbar-app/src/row_layout/project_switcher_panel.rs`.

**No live reference.** This item does not run the oracle or capture a
snapshot — per the item brief's hard constraints. Every number below is read
off the app's own compiled Tailwind (`native/MAPPING.md`'s method) or
transferred from an existing measurement, not off a live capture.

## 0. What this file is, and what it is not

`project-switcher-panel.tsx` is `ProjectSwitcherPanel()`: the body of the
pushed "Projects" sidebar screen — `nav-stack.tsx` supplies the back button
and screen title around it, out of this item's own scope (`nav-stack` is its
own, separately-flagged Tier B target per
`native/mapping/layout-denominator.md` §4). The body itself is a column of
project rows (each the project's own name, tinted when it is the active
one) followed by one static "Import project" row. It composes
`<ImportProjectModal>` unconditionally (starting closed) — that dialog lives
under `components/projects/`, outside this item's own scope, so the port
does not compose it. See §5.

Confirmed **LIVE** by `native/mapping/liveness-audit.md`'s own method:
`project-home-row.tsx` pushes it onto the nav stack on click.

## 1. Values

| React / Tailwind | Compiles to | gpui / `crowbar-ui` |
|---|---|---|
| `py-1` (row-list wrapper) | 4px | `WRAPPER_PADDING_Y` |
| `h-9 px-1.5 gap-1.5 rounded-lg border text-[13px] font-medium` (`ROW_BASE`, both row kinds) | — | `row_base::base` — see `project-home-row.md` §1 for the same table, not repeated here |
| `mx-1.5` / `my-0.5` (`ROW_BASE`, both row kinds) | 6px / 2px | `row_base::MARGIN_X` / `MARGIN_Y` — **applied here**, unlike `project-home-row`. See §2. |
| `border-transparent text-left hover:bg-accent` (project row, resting) | — | `Color::TRANSPARENT` border, `theme.foreground` text |
| `bg-accent/60 text-foreground` (project row, active) | — | `theme.accent.mix(60.0, TRANSPARENT)` |
| `border-transparent text-muted-foreground hover:bg-accent hover:text-foreground` (import row) | — | `theme.muted_foreground` text, transparent border |
| `<Plus className="size-3.5 shrink-0" />` | 14px, unpainted | `PLUS_GLYPH_SIZE` |
| `hover:*` (both row kinds) | colour-only | not modelled — no runtime seam, `row_base.rs`'s own module docs |
| `h-full` (outer wrapper) | — | **not modelled** — see §3 |

## 2. `mx-1.5`/`my-0.5` *are* applied here, unlike `project-home-row`

`row_base.rs`'s own module docs record the general rule: a row captured as
its own standalone root has no anchor its own margin could move
(`getBoundingClientRect()`/a gpui anchor's own bounds both exclude an
element's own margin), so `project-home-row.rs` does not apply it. This
surface is the other case: every row here sits beside siblings inside **one
captured list**, and the margin *is* the actual spacing mechanism between
them — verified in `row_layout.rs`'s own
`rows_stack_with_the_wrappers_padding_and_each_rows_own_margin`, which reads
the real stride (`row_base::HEIGHT + row_base::MARGIN_Y * 2.0`) off a real
taffy layout. Flex items do not collapse margins the way block siblings can
(margin collapsing is a block-formatting-context rule; `ROW_BASE`'s own
list wrapper is `flex flex-col`), so the visible gap between two adjacent
rows is the full `MARGIN_Y * 2`, not `MARGIN_Y` — reproduced exactly by
applying `.my(MARGIN_Y)` to each row rather than a `gap` on the container
(the source has no `gap-*` on `flex flex-col py-1` at all).

## 3. `h-full` on the root is not modelled

`project-switcher-panel.tsx`'s own outer `<div className="flex h-full
flex-col">` stretches to whatever height its `NavStack` parent gives it.
Measured against this port's own row-layout harness (`row_layout.rs`'s
`Stage`): the immediate parent of a surface's root is `div().w(width_px)`
with **no height style at all** — `display: block`, auto height — so a
percentage height here resolves the way CSS resolves any percentage against
an indefinite-height containing block: as `auto`. `h-full` therefore
collapses to exactly the same picture as omitting it in this harness, and
this port omits it rather than call `.h_full()` for a property that would
be a no-op here and a live risk (an indefinite ancestor resolving a
percentage to `0` instead of `auto`) anywhere the assumption does not hold.
`ProjectSwitcherPanel::content_height()` is what drives the surface's own
window height instead — the same shape `command.rs`'s own `popup_height` is
for its own content-driven popup.

## 4. Row ids are index-parameterized, not fixed strings

The project list's length varies (zero projects up through however many
are open), and `native/oracle/ANCHORS.md` v1.8 refuses a capture where the
same anchor id appears twice under one root — `sidebar-project-header.tsx`'s
own four-button fix, needed here because the row *count* varies rather than
a fixed small set of named buttons. Each row's own id is
`project-switcher-panel-row-{index}` (and its label,
`…-{index}-label`), built the same way on both sides: a template literal in
the React `.map((p, index) => …)` callback, and
`crowbar_ui::components::project_switcher_panel::{row_id, row_label_id}` on
the Rust side. There is no static `LINE_SIZED` array naming the per-row
label ids for this reason — a fixed-size array cannot enumerate a
parameterized family — but every row's own label is still declared
`line_sized` at its own call site regardless.

## 5. `ImportProjectModal` is not composed

Starts closed, lives under `components/projects/`, outside this item's own
scope — the same `sidebar_carousel.rs` posture `project-home-row.md`'s own
§0 already cites for `AddRepositoryModal`. This panel's own painted picture
never depends on whether the dialog is open.

## 6. Anchoring

`project-switcher-panel.tsx` carried no `data-oracle-id` before this item.
Added:

* `project-switcher-panel` — the outer `<div className="flex h-full
  flex-col">`, this surface's own root.
* `project-switcher-panel-row-{index}` / `-{index}-label` — one pair per
  project row, index-parameterized (see §4).
* `project-switcher-panel-import` / `-import-label` — the static "Import
  project" row and its label.

**No `oracleSurfaceScope` entry needed.** Every anchor in this composition
is content this file itself paints — plain `<button>`s, not composed
surfaces the way `project-home-row.tsx`'s `<WorkspaceBranchIcon>` is (see
`project-home-row.md`'s own scope-entry account for the shape that *would*
need one).

## 7. Declarations

`CONTENT_SIZED = []` — every label is `flex-1` (sized by its row, not by
its own text). `LINE_SIZED = [project-switcher-panel-import-label]` as a
static array; every project row's own label is *also* `line_sized` at its
own call site, just not nameable in a fixed-size array — see §4.

## 8. The state axis

| flag | here |
|---|---|
| `empty` | **real.** Zero projects is a genuinely reachable picture (a fresh install with nothing imported yet), and it swaps `ProjectSwitcherPanel::rows` for an empty list, leaving only the always-present import row. The same shape `command.rs`'s own `Empty`/`Item` swap is. |
| `loading`, `error` | unmodelled (mandatory on every surface). |
| `hover` | unmodelled — colour-only, no runtime seam. |
| `focus`, `selected` | unmodelled — `aria-current` is an accessibility attribute, not a CSS selector target anywhere in either row's class strings; nothing styles it. |

`Params::no_state_axis()` returns `false` — one real flag (`empty`).

## 9. `row_layout` coverage

* the default cell (two projects) carries both rows, their labels, and the
  import row + label, and never a third project row
* `--flags empty` carries only the import row, regardless of `--count`
* `--count` drives the row count exactly, past the fixture-name list's own
  length (names cycle)
* the first row sits at the wrapper's own `py-1` plus its own `my-0.5`; each
  following row (including the import row) follows by exactly one more
  `row_base::HEIGHT` plus two `row_base::MARGIN_Y`s
* every row is inset by `row_base::MARGIN_X` on both edges
* the root's own width tracks `--width` exactly

## 10. Reachability

`project-home-row.tsx` → `useSidebarNavStore.getState().push({ …component:
<ProjectSwitcherPanel /> })`, rendered by `nav-stack.tsx` once pushed.
`project-home-row.tsx` is itself reachable per `project-home-row.md` §10, so
this panel is one click away from the resting sidebar.

---

## VERDICT: FAIL — 5 deltas over 5 anchors, **1 real defect** (2026-08-03, my own run)

Drive: `--surface project-switcher-panel --width 344 --viewport-width 1200
--theme dark --content normal --count 1 --active-index 0`, against the panel
opened by clicking `project-home-row-switch`.

```
row-0-label.text:          "crowbar", expected "oracle-fixture"  (exact)
panel.bounds.h:            88.0,      expected 756.0             (Δ -668.0)
row-0-label.text_width:    54.6,      expected 109.2             (Δ -54.6)
import-label.text_width:   88.959,    expected 90.96             (Δ -2.001, tol ±1.0)
import-label.font.weight:  400,       expected 500               (exact)
```

### ✅ First, what this run CONFIRMS — P3.60 on its second consumer

**`font.line_height` does not appear in the delta list at all.** Both labels
match at **19.5**, and the label boxes (`h`, `y`) are inside tolerance. This
surface is the *other* consumer of `row_base::LINE_HEIGHT_RELATIVE`, and it was
never driven when that constant was fixed — so this is the first independent
check of it.

It also settles the question the P3.60 worker raised and answered on reasoning
alone: the **"Import project" label is CalSansUI, not `font-mono`**, and it
reports `line_height: 19.5` exactly like the mono label beside it. Unitless
line-height really is font-*size*-driven, not font-*family*-driven. No family
split was needed, and now that is measured rather than argued.

### The one real defect

**`import-label.font.weight` — 400 against 500.** §1's own table records
`ROW_BASE` as `… text-[13px] font-medium` for **both row kinds**, so the import
row should be 500 like the project row (which is correct at 500). The port
drops the weight on this one label.

**`import-label.text_width` is the same defect, not a second one**: 88.959
against 90.96 is a 2px shortfall on a 14-character string, which is what
shaping the same text at 400 instead of 500 produces. Fixing the weight should
close both.

### The two fixture gaps

`row-0-label.text` (`"crowbar"` vs `"oracle-fixture"`) and its `text_width` are
one gap: **`--project-name` exists on `project-home-row` but not on this
surface**, so no drive can make the fixture say what the live app says. Third
surface today blocked on a hard-coded fixture string (`repo-icon-popover`'s
`"R"`, `repo-avatar`'s `"RE"`). These want one flag each.

### ⚠ `panel.bounds.h` — §3's reasoning describes the fixture, not the app

§3 above argues `h-full` is unmodelled because the parent has "**no height
style at all** — `display: block`, auto height — so a percentage height
resolves … as `auto`". **That is true of the context §3 was written against and
false of the running app**: live, `NavStack` gives the panel a definite height
and `h-full` stretches it to **756px** against the port's content-sized 88.

So this is not the port miscomputing a height — it is the port modelling a
containing block the app does not have, and the surface having **no height
axis** to express the one it does. Same shape as `repo-import-dialog`'s
`--window-height`, which exists precisely because that surface's height comes
from outside it. This surface needs the equivalent before its root's `h` can be
compared at all.

Until then the root's height delta is **not evidence about the port**, and
should not be counted as one.
