# `project-home-row` (P3.60)

`web/src/components/layout/project-home-row.tsx` →
`crates/crowbar-ui/src/components/project_home_row.rs`,
`crates/crowbar-app/src/surfaces/project_home_row.rs`,
`crates/crowbar-app/src/row_layout/project_home_row.rs`.

**VERDICT: PASS — 0 deltas over 5 anchors compared** (2026-08-03, my own
side-by-side run; the drive is in §3.1).

The *worker* did not run the oracle — that is the item brief's hard
constraint and it held. Every number below was originally read off the app's
own compiled Tailwind (`native/MAPPING.md`'s method) or transferred from an
existing measurement. **One of those transfers was wrong, and only the live
run found it** (§3). This header used to read "No live reference. This item
does not run the oracle" and stayed that way after the verdict was taken —
a stale disclaimer sitting directly above a section reporting live oracle
output. Corrected here.

## 0. What this file is, and what it is not

`project-home-row.tsx` is `ProjectHomeRow()`: the sidebar row for the active
project's "home" — a 20px identity glyph (the `Library` house-of-books mark,
or the agent spinner while the home workspace is working), the project's own
name (or the literal `'home'` fallback), and two trailing icon-only actions
(import a repository; open the project switcher). It composes
`<AddRepositoryModal>` unconditionally (starting closed) — that dialog lives
under `components/projects/`, a different directory this item's own scope
(`components/layout/`) does not reach, so the port does not compose it. See
§4.

Confirmed **LIVE** by `native/mapping/liveness-audit.md`'s own method:
reached via `workspace-tree.tsx`, which is itself part of the always-mounted
sidebar column.

## 1. Values

| React / Tailwind | Compiles to | gpui / `crowbar-ui` |
|---|---|---|
| `h-9` (`ROW_BASE`) | 36px | `row_base::HEIGHT` |
| `px-1.5` (`ROW_BASE`) | 6px | `row_base::PADDING_X` |
| `gap-1.5` (`ROW_BASE`) | 6px | `row_base::GAP` |
| `mx-1.5` / `my-0.5` (`ROW_BASE`) | 6px / 2px | **not modelled on this row** — see §2 |
| `rounded-lg` (`ROW_BASE`) | `theme.radius_lg` (10px, not Tailwind's stock 8 — `theme.css` redefines it) | reused, not re-derived |
| `border` (`ROW_BASE`) | 1px, unconditional | `button::BORDER_WIDTH` — `button.rs`'s own headline finding reproduced here |
| `text-[13px] font-medium` (`ROW_BASE`) | 13px font, `19.5px` line height (`1.5` unitless, Tailwind's own preflight default — **not** `normal`) | `row_base::TEXT` / `row_base::LINE_HEIGHT_RELATIVE` — see §3 |
| `border-background bg-background text-foreground` (`ROW_ACTIVE`) | — | `row_base::active` |
| `border-transparent text-foreground` (`ROW_INACTIVE`) | — | `row_base::inactive` |
| `shadow-xs shadow-black/10`, `inset-shadow-[…]` (`ROW_ACTIVE`) | box-shadows | not modelled — `ANCHORS.md` §6 has no field |
| `hover:bg-accent` (`ROW_INACTIVE`) | colour-only | not modelled — no runtime seam on this struct, see §6 |
| `h-5 w-5` (icon wrapper) | 20px | `ICON_WRAPPER_SIZE` |
| `<Library size={16}>` | 16px, unpainted | `LIBRARY_GLYPH_SIZE` |
| `size-7 rounded-md sm:size-6` (`icon-xs` variant, before the call site's override) | 28px→24px, `theme.radius_md` (8px) | superseded — see §4 |
| `cn(ROW_SUB_ACTION, 'size-6')` (call site) | **24px at every width**, `rounded-lg` (10px) | `row_base::SUB_ACTION_SIZE`, `button::RadiusClass::Lg` — see §4 |
| `p-1.5` (`ROW_SUB_ACTION`) | 6px | not applied as padding; achieved by centring a `size-3` glyph in the fixed 24px box instead (same visual result) |
| `size-3` (glyph inside each action button) | 12px, unpainted | `row_base::SUB_ACTION_GLYPH` |
| `text-muted-foreground` (`ROW_SUB_ACTION`) | — | `theme.muted_foreground` |

## 2. `mx-1.5`/`my-0.5` are not modelled on this row

`ROW_BASE`'s own outer margin only has somewhere to show up in the context of
a *surrounding list*. `project-home-row.tsx` is captured as its own
standalone row — the same shape `sidebar_project_header.rs` and
`context_pill.rs` already are — and a captured element's own bounds
(`getBoundingClientRect()` on the React side, a gpui anchor's own bounds on
this one) exclude the element's own margin on both sides of the port
regardless. So there is no anchor these two lengths could move here, and
`crowbar_ui::components::row_base::base` does not apply them.

**This is not true of every consumer of `ROW_BASE`.** `project-switcher-
panel.rs` — the sibling item in this same wave — *does* apply
`row_base::MARGIN_X`/`MARGIN_Y` to each of its own rows, because there each
row sits beside siblings inside one captured list, and the margin is the
actual spacing mechanism between them. `row_base.rs`'s own module docs
record both halves of this so a future consumer does not have to re-derive
which case it is in.

## 3. `text-[13px]`'s own line height — wrong on the first pass, caught by the oracle

**This section originally said `text-[13px]` carries no paired
`line-height` utility, so its box is CSS `normal`, resolved through the
font's own metrics, and transferred `row_base::LINE_HEIGHT_RELATIVE` from
`context_pill::LARGE_LINE_HEIGHT` (18px) on that basis. That was wrong.**
P3.60's own parity run against the live app reported it directly:

```
project-home-row-label.bounds.h:         18.0, expected 19.5   (Δ -1.5, tol ±0.5, line_sized)
project-home-row-label.font.line_height: 18.0, expected 19.5   (Δ -1.5, tol ±0.5)
```

Both reference fields agree at 19.5 — this is **not**
`context_pill.rs`'s own trap (a computed-style value disagreeing with the
rendered box); the reference was never in doubt, this port's own number
was. `19.5 = 13 × 1.5`, and `1.5` is not `normal` at all: Tailwind's own
preflight sets `html { line-height: 1.5; }`
(`node_modules/tailwindcss/preflight.css:30`), a **unitless** ratio that is
inherited and recomputed against each descendant's own font-size. Neither
`ROW_BASE` nor either label span in this file (or `project-switcher-
panel.tsx`'s) sets a `leading-*` class or a paired `text-*` size utility to
override it, so the inherited `1.5` reaches the label unchanged —
`13 × 1.5 = 19.5px`, matching the reference on both fields.

`row_base::LINE_HEIGHT_RELATIVE` is now `1.5`, with the full derivation and
correction in its own doc comment. It applies identically to
`project-switcher-panel.tsx`'s two label shapes too: unitless
`line-height` is font-*size*-dependent, not font-*family*-dependent, so the
`font-mono`/default-sans split between that surface's two row kinds does
not create a second number to derive. `context_pill.rs`'s own
`LARGE_LINE_HEIGHT` sits under a different ancestor chain and is not
re-derived by this fix — only this file's claim on the number, and
`row_base.rs`'s shared constant, were wrong.

### 3.1 The confirming run, and how I nearly read it backwards

Post-fix, both sides at `width=855 theme=dark content=normal
flags=[selected]`:

```
oracle: project-home-row · width=855 theme=dark content=normal flags=[selected]
oracle: PASS — 0 deltas over 5 anchors compared
```

Independently corroborated in the live DOM before the differ ran —
`getComputedStyle(label).lineHeight` is **`19.5px`** at `fontSize: 13px`, so
the ratio really is `1.5` and the fix is not merely a number that makes the
differ happy.

**Three method traps fired on this one run**, all mine, none the port's:

1. **A pre-fix *native* capture was sitting in my scratchpad under a name
   that read like a reference.** It reported the label at `18.0`, which is
   the exact opposite of the recorded verdict (`18.0, expected 19.5`), and
   for a moment I believed I had merged a regression. `diff.rs`'s own doc
   settles the direction — `expected` is React, `actual` is native — but the
   file itself carried nothing saying which side produced it. **A capture
   file has to name its own side**; `native.json`/`ref.json`, never
   `phr-selected.json`.
2. **`CROWBAR_ROW_SNAPSHOT` is required, and omitting it is indistinguishable
   from omitting `--features driver`.** Both leave `crowbar-app` sitting in
   an ordinary window that never exits. I had the feature right and the
   destination missing, and spent four rebuilds re-diagnosing a hang I have
   already been bitten by once. `oracle/README.md` now names both gates
   together.
3. **`extractSnapshot` defaults `state.width` to the root's own width** when
   the caller passes no `width`. That silently produced `width: 332` — the
   surface, not the viewport (855) — which is the `--width`/`--viewport-width`
   confusion arriving from the React side for the first time. Pass
   `window.innerWidth` explicitly.

## 4. `size-6` wins over the `icon-xs` variant's own box at every width

`button.tsx` composes `cn(buttonVariants({ className, size, variant }), …)`,
so `cn(ROW_SUB_ACTION, 'size-6')` is merged **over** `icon-xs`'s own
`size-7 sm:size-6`. tailwind-merge groups a class by its own variant scope:
the call site's un-prefixed `size-6` conflicts with (and replaces)
`icon-xs`'s own un-prefixed `size-7`, but does **not** touch `icon-xs`'s
`sm:size-6` (a different scope). The merged result is `size-6 sm:size-6` —
**24px at every width**, not the 28→24 step every other `icon-xs` button in
this port takes. This is why the two trailing-action buttons (and the row
shell itself) need no `Breakpoint` parameter at all: nothing on this
surface carries an `sm:` variant of its own once the merge is worked
through.

`rounded-lg` wins over `icon-xs`'s own `rounded-md` the same way
(`button.rs`'s own finding for the workspace header's two `icon-xs`
buttons — host **10px**, `::before` **7px**; irrelevant here beyond the
host radius, since this hand-built box paints no `::before` overlay at
all).

## 5. This composition does not call `Button::render`

`Button::render` calls `anchors.root(id, …)` — its own frame boundary,
wrong for two buttons inside this surface's single root. Exactly as
`context_pill.rs` and `sidebar_project_header.rs` already do, this port
hand-builds each button's box off `button::Size`/`RadiusClass`'s own public
values (via `row_base::sub_action_box`) rather than reusing `Button::
render`'s anchor machinery.

## 6. Anchoring

`project-home-row.tsx` carried no `data-oracle-id` before this item. Five
are added:

* `project-home-row` — the outer `role="button"` `<div>`, this surface's own
  root.
* `project-home-row-icon` — the 20px identity-glyph wrapper `<span>`,
  present regardless of which picture it holds.
* `project-home-row-label` — the truncating project-name `<span>`, carrying
  `data-oracle-line-sized="true"` alongside it (v1.6 — see §7).
* `project-home-row-import` / `project-home-row-switch` — the two trailing
  `<Button>`s, overriding `button.tsx`'s own `'data-oracle-id': 'button'`
  default (the same fix `sidebar-project-header.tsx`'s four buttons already
  needed — two Buttons in one row means the shared default would collide
  with itself, let alone with the generic `button` surface).

**`<Library size={16} />` and the two trailing glyphs (`FolderSymlink`,
`LayoutGrid`) are deliberately *not* separately anchored.** Each sits inside
a wrapper that already carries an id and is always present regardless of
which picture the wrapper holds — the restraint `context_pill.rs`'s own
label lines already take, not anchoring every nested text run or glyph.

### The scope-entry decision, argued in full

**`project-home-row` composing `workspace-branch-icon`: the port paints the
icon, so it is composed content and needs no `oracleSurfaceScope` entry.**

`ProjectHomeRow::icon()` calls `WorkspaceBranchIcon::render(theme, anchors)`
directly on the `working` branch — the identical composition
`context_pill.rs` already makes for its own `Workspace`/`Home` variants.
`WorkspaceBranchIcon::render` opts its own anchor via `anchors.boxed(…)`,
never `.root(…)`, so nesting it inside this surface's own root is
collision-free: every anchor a live capture of this row could reach
(`project-home-row-icon`, and, one level deeper on the working branch,
`workspace-branch-icon` and, deeper still, `flicker-spinner`) is exactly
what this composition paints. Nothing is left for the differ to find that
this port did not itself put there.

This is the opposite shape from `sidebar-project-header.tsx`'s toggle icon,
which is the worked example the item brief points at: that composition's
own `<SidebarToggleIcon />` position is left as an **empty placeholder
box** — the real glyph is a separately-ported, separately-anchored
primitive the port does not reach into — so a live capture of
`sidebar-project-header` would carry a `sidebar-toggle-icon` id nested
inside it that the Rust port's own tree never produces at all, and the
scope entry is what tells the differ that nested id is foreign, not a
missing anchor.

`project-home-row` never has that gap. The `Library` glyph on the idle
branch is likewise painted directly by this file (an empty, unpainted box —
no native equivalent, the same call every icon in this port makes), not
left to some other surface to fill in. **Composed-and-painted, not
foreign-and-unpainted — no entry.**

## 7. Declarations

`CONTENT_SIZED = []` — the label's own box is `flex-1` (sized by the row,
not by its own text). `LINE_SIZED = [project-home-row-label]` — the label
is a blockified flex item in an `items-center` (not `stretch`) row with no
explicit height of its own, so its box *is* its own line box regardless of
the row's authored `h-9` — the same shape `git_status_row`'s `ID_NAME`
already is. `data-oracle-line-sized="true"` is declared on the React side
to match.

## 8. The state axis

| flag | here |
|---|---|
| `selected` | **real**, via `--flags selected` / `StateFlag::Selected`. `isActive` (`useMatch`) selects `row_base::active` over `row_base::inactive` — a single row's own active/idle picture is exactly the `` `data-active='true'` `` concept `ANCHORS.md` v1.1 names. |
| `empty` | unmodelled — `StateFlag::Empty` is `git-status-row`'s own "nothing on the trailing edge" concept; this row always paints both its glyph and its two actions. |
| `loading`, `error` | unmodelled (mandatory on every surface). |
| `hover`, `focus` | unmodelled — colour-only rules (`ROW_INACTIVE`'s `hover:bg-accent`, `ROW_SUB_ACTION`'s `hover:…`, both buttons' `focus-visible:ring-…`) with no runtime seam on this struct, `row_base.rs`'s own module docs record why. |

`Params::no_state_axis()` returns `false` — one real flag (`selected`).

## 9. `row_layout` coverage

* the default cell carries all five anchors and never the working-only ones
* `--working` swaps the icon for `workspace-branch-icon` (+ nested
  `flicker-spinner`)
* the root's own height stays the authored `row_base::HEIGHT` whether or not
  `selected` — colour is the only field `selected` moves, outside what
  `row_layout` asserts (bounds only, matching every other row-shaped
  surface in this port)
* the icon sits flush against `row_base::PADDING_X`; the label follows by
  `row_base::GAP`
* the label's own line box is `13 × row_base::LINE_HEIGHT_RELATIVE` (19.5px)
  — added after the live oracle caught this constant at the wrong value; no
  assertion here had checked the label's own height before, which is why a
  wrong ratio passed every gate in this file (see §3)
* the root's own width tracks `--width` exactly

## 10. Reachability

`workspace-tree.tsx` → `sidebar-carousel.tsx` → `ide-shell.tsx` →
`routes/_shell.tsx`. Always mounted above the repo tree, per
`native/mapping/layout-denominator.md`'s own table (`project-home-row.tsx`,
119 lines, sole importer `workspace-tree.tsx`).

---

## ❌ VERDICT — FAIL, 2 deltas over 5 anchors (2026-08-03, taken by me)

```
project-home-row-label.bounds.h:         18.0, expected 19.5   (line_sized)
project-home-row-label.font.line_height: 18.0, expected 19.5
```

**Two lines, one defect.** Everything else matches to the pixel: root `332×36`
with `bg`/`border` `#1f1f1eff`, icon `20×20` at `(7,8)`, both buttons `24×24` at
x271/x301, and `text_width` **109.2** exactly.

`19.5` is `13 × 1.5`. Because the anchor is `line_sized`, the differ compares
`bounds.h` against the reference's **`font.line_height`**, not its `bounds.h`
of 19.0 — so both deltas close together.

**The font fix is confirmed by a surface it was not written for**: the reference
reports `font.family: "JetBrains Mono Variable"` and the port matches it, with
`text_width` agreeing to 0.0. P3.57 registered that face for `fps-overlay`.

### The drive — ANCHORS **v1.14** — and three driving errors of mine

```
reference:  live Tauri app, route #/ide/<id>/home, dark, viewport 1714,
            project row ACTIVE (isActive true on the home route),
            project name "oracle-fixture"
native:     crowbar-app --surface project-home-row --project-name oracle-fixture \
                        --width 332 --viewport-width 1714 --theme dark --flags selected
```

I got there through three wrong cells, and each was caught by a different guard:

1. **No `--project-name`** → `text: "home"` against `"oracle-fixture"`, and a
   78px `text_width` delta following from it. Caught by the **text** comparison.
2. **`flags: []` in the reference I wrote**, against a row that is visibly
   active → the differ **refused outright**: *"the two snapshots are not the same
   §8.3 matrix cell… comparing across them is the easiest possible way to fake
   convergence."* The state block is mine to fill and I filled it wrong; the
   refusal is the guard working on its author.
3. Consequently two spurious **colour** deltas (`bg`, `border.color`) that
   vanished the moment the cell was right.

**Four of the six original deltas were mine.** The surviving two are the port's.

---

## ⛔ REGRESSED (2026-08-04) — a shared-helper fix elsewhere broke this surface's PASS

P3.66 fixed a real phantom-border defect on `repo-section-import`/
`-collapse` and `workspace-tree-item-add-child` by removing
`.border(button::BORDER_WIDTH).border_color(Color::TRANSPARENT)` from
`row_base::sub_action_box` **globally**. Correct for those three — **and
wrong for this surface**, whose two trailing actions genuinely carry that
same border in the live DOM: `PASS 0/5` → `FAIL 2/5`.

```
project-home-row-import.border.w:  0.0, expected 1.0
project-home-row-switch.border.w:  0.0, expected 1.0
```

## FIXED (P3.81)

**Root cause, established from the React source rather than assumed:** this
surface's two actions are **not** the same shape as `repo-section.tsx`'s and
`workspace-tree-item.tsx`'s own trailing actions. Those two render raw
`<button className={ROW_SUB_ACTION}>` elements — no `<Button>` primitive at
all — and `ROW_SUB_ACTION`'s own class list (`workspace-row-base.ts`) never
carries a `border` utility, so P3.66's removal is correct for them.
`project-home-row.tsx`'s two actions are `<Button variant="ghost"
size="icon-xs" className={cn(ROW_SUB_ACTION, 'size-6')}>` — the **shared**
`Button` primitive, whose base class (`button.tsx`, `buttonVariants`)
carries a bare, unconditional `border` (`button.rs`'s own headline finding).
`ROW_SUB_ACTION` has no `border`/`border-color` utility to override it with,
so tailwind-merge leaves the primitive's own `border border-transparent`
(`ghost`'s own colour) in the final class string untouched. **One shared
`row_base::sub_action_box`, two different DOM shapes underneath it** — a raw
element with no border ever, and a `<Button>` whose border only its colour
varies.

Fixed by restoring the border **at this surface's own call site**
(`ProjectHomeRow::sub_action`,
`crates/crowbar-ui/src/components/project_home_row.rs`) rather than putting
it back on the shared helper: `.border(button::BORDER_WIDTH)
.border_color(Color::TRANSPARENT)` chained onto `row_base::sub_action_box`'s
own result, mirroring `Button::render`'s own `shell()` (`.border_1()` +
`Variant::Ghost`'s own `Color::TRANSPARENT`). `row_base::sub_action_box`
itself is untouched and stays correct — no border — for its other two,
genuinely-raw-`<button>` consumers.

### The full `sub_action_box` consumer list, and what each paints

| consumer | React shape | `border.w` |
|---|---|---|
| `repo-section-import`/`-add-child`/`-collapse` | raw `<button className={ROW_SUB_ACTION}>` | **0** — unchanged by this fix |
| `workspace-tree-item-expand`/`-add-child` | raw `<button className={ROW_SUB_ACTION}>` | **0** — unchanged by this fix |
| `project-home-row-import`/`-switch` | `<Button variant="ghost" size="icon-xs">` merging `ROW_SUB_ACTION` | **1**, `#00000000` — restored by this fix |

No fourth consumer exists (`grep -rn "sub_action_box"` over `native/`,
excluding `target/`, turns up exactly these three files' six call sites —
`crates/crowbar-app/src/row_layout/{workspace_tree_item,repo_section}.rs`
only *mention* the function in doc comments, they do not call it).

### Regression guarded

`crates/crowbar-app/src/row_layout/project_home_row.rs` gained
`the_two_actions_paint_a_transparent_border`, run against a real reversion
of the fix (removed the `.border(...).border_color(...)` chain) and
confirmed to fail (`expected 1px, got 0px`) before being reverted back to
the fix.

### `workspace-tree` inherits this fix with no code change of its own

`native/mapping/workspace-tree.md`'s own two `FAIL` survivors
(`project-home-row-import`/`-switch` `border.w`) are this exact regression,
reached through the row it composes — closed by this same fix, nothing
further needed in `workspace_tree.rs`.
