# `search` (P3.33) — an ordinary port, built not wrapped, that overturns a `button.rs` finding

`web/src/components/ui/search.tsx`'s three exports (`SearchPopover`,
`SearchReplaceToggle`, `SearchReplaceRow`) →
`crates/crowbar-ui/src/components/search.rs`.

> A §6.2 row, in the shape `native/MAPPING.md` fixes. Kept in its own file for
> the reason `popover.md` gives.

## 0. The brief's own premise, checked first

The queue previously flagged `search` as needing Zed's `fuzzy_nucleo` (§10.1).
**Checked again for this item and it is still wrong, for the reason
`native/QUEUE.md`'s own correction already gives**: `grep -n
'\.filter(\|score\|fuzzy\|match(\|includes(\|indexOf'
web/src/components/ui/search.tsx` returns **0 lines**. The file is a
`Button` + `Input` shell and nothing else; the real matchers
(`web/src/utils/search-match.ts`, `web/src/utils/fuzzy-matcher.tsx`) live
outside `components/ui/` and are Phase 4 work. **No matching logic was found
in `search.tsx`** — the brief's contingency for "stop and report" does not
apply; this confirms the measurement rather than contradicting it.

**`search-toggle-icons` is a different surface, not the same one as
`SearchReplaceToggle`.** `search-toggle-icons.tsx` is a `Record` of four
`ReactNode`s (three phosphor glyphs plus one `Aa` span) used as the `icon`
prop of `SearchPopover`'s `options` array — it is content that goes *inside*
one of this surface's toggle buttons. `SearchReplaceToggle` is a wholly
distinct, separately-exported component: the chevron button that expands or
collapses the replace row, passed as `SearchPopover`'s `leadingControl` prop.
The two share no code, no class list and no anchor id; `search-toggle-icons`
(P3.8) is already ported and verified (PASS, re-verified after the weight
fix per `native/QUEUE.md`'s Wave 5 sweep) and is reused **unmodified** here —
`crowbar_ui::components::search_toggle_icons::SearchToggleIcon` is called
directly from inside this surface's toggle-button shell, exactly as
`search-toggle-icons.rs`'s own module docs already anticipated
(`glyph_extent`/`inherited_line_height` are both written in terms of
`SearchPopover`'s host button, by name, from that earlier item).

**Importer count.** The brief states 3; measured, it is **2**:
`web/src/features/editor/components/toolbar/find-bar.tsx` and
`web/src/features/terminal/components/terminal-search.tsx`, both importing
`SearchPopover` (and `find-bar.tsx` additionally `SearchReplaceRow` and
`SearchReplaceToggle`). No third importer exists —
`git grep "from '@/components/ui/search'"` and every relative-path spelling
both return exactly these two files. A minor correction to the brief's own
count, recorded rather than silently reconciled; it does not change scope.

## 1. Built, not wrapped — the seam test, applied

> A widget is wrappable-and-measurable exactly when it lets the caller supply
> an *element*, not merely a style.

`search.tsx` renders through **no `gpui-component` primitive at all** —
`grep -c 'gpui_component\|@radix-ui\|@base-ui'` on the file is zero. Unlike
`dropdown`/`popover` (wrap `gpui_component::popover::Popover` for deferred
positioning) or `tooltip`/`select` (built because the vendor's own render
path has no seam), `search.tsx` has no vendor widget to test the seam
against in the first place: every box is `search.tsx`'s own `<div>`s,
`<Button>`s and one `<Input>`, and the popup's *position* is the caller's
concern (`find-bar.tsx`'s own `absolute top-9 right-2`), not this file's.
There is no `render`/`children` prop on anything here that a vendor owns —
so the question "does the seam let the caller supply an element" has no
subject. **Verdict: built**, trivially — every `Div` this module constructs
is already `crowbar-ui`'s own to anchor, because nothing outside this crate
built any of them.

`SearchReplaceToggle`'s and `SearchReplaceRow`'s buttons are **not** routed
through `crowbar_ui::components::button::Button::render`, for a second,
sharper reason than "built not wrapped": that API's `Label` enum paints a
*closed set of hard-coded strings* (`"Add"`, `"Create workspace"`, `"Import
an existing repository"`) chosen for `button`'s own captured cells, with no
parameter for an arbitrary caller string — so it cannot paint `"Replace"`/
`"All"` verbatim regardless of wrap-vs-build. Each icon-shaped button here is
this module's own small `Div`, built with `button`'s **sealed tokens**
(`theme.radius_lg`, `button::DISABLED_OPACITY`, `button::Size::Default`'s
icon-sizing table) held *by reference*, the same "own copy, shared tokens"
relationship `alert_dialog` keeps with `dialog`, and `dropdown::trigger`/
`item` keep with nothing at all.

## 2. The size-6/`sm:h-8` trap — `native/MAPPING.md`'s third occurrence, confirmed in both directions live

Every icon-shaped button on this surface (`searchIconButtonVariants`,
`searchToggleButtonVariants`) is `flex size-6 items-center justify-center
rounded-lg border border-transparent …`, merged via `cn(buttonVariants({
className, size, variant }), …)` onto a bare `<Button variant="ghost"
compact>` — **no `size` prop**, so the base list underneath is
`Size::Default`'s own `h-9 sm:h-8`.

`size-6`'s unprefixed height component and `buttonVariants`' unprefixed
`h-9` are the *same* tailwind-merge class group with the *same* (absent)
modifier, so `size-6` **replaces** `h-9` outright when the merge runs. But
`sm:h-8` carries a **different** modifier and survives untouched — and its
media-scoped rule sits later in Tailwind's generated stylesheet than
`size-6`'s plain one, so at a `sm` viewport it **wins the cascade** despite
losing the merge.

| viewport | which rule wins the height | measured box |
|---|---|---|
| ≥ 640px (`sm`) | `sm:h-8` (media-scoped, later in the sheet) | **24 × 32** |
| < 640px | nothing competes with `size-6` | **24 × 24** |

Both rows measured live, not reasoned through in one direction only:

* **1714px window**, editor find bar open — every icon-shaped anchor
  (`search-replace-toggle`, `search-close`, all three option toggles, both
  nav chevrons) read `24×32` via `getBoundingClientRect`, confirmed a second
  way by `extractSnapshotSource`'s own capture (`/tmp/p3-ref-search.json`).
* **550px window** (below the 640px `sm` boundary; `window.innerWidth` read
  550 via `execute_js`), the same find bar reopened at that width — the
  leading toggle and the visible option toggle both read `24×24`.

`crowbar_ui::components::search::icon_button_extent` is exactly this pair,
and its `Sm` arm is spelled `button::Size::Default.extent(Breakpoint::Sm)`
rather than the literal `32.0` — read off `button`'s own table so a future
change to its `sm:h-8` moves this surface with it, the same discipline
`search_toggle_icons::glyph_extent` already established for the *icon inside*
these same buttons.

## 3. Values — the popover shell

Measured live on `/tmp/p3-ref-search.json`'s own cell (1714px, dark,
resting, replace collapsed, empty query).

| React / Tailwind | Compiles to (measured) | gpui / `crowbar-ui` | Oracle |
|---|---|---|---|
| `w-[320px]` | `width: 320px`, fixed | `POPOVER_WIDTH` | `bounds.w` = 320 |
| `rounded-xl` | `border-radius: 14px` | `theme.radius_xl` | `radius` = 14 |
| `border border-border/70` | `border-width: 1px`, `#ffffff0b` | `theme.border.mix(70.0, TRANSPARENT)` | `border.w` = 1, `border.color` |
| `bg-background/95` | `#1f1f1ef2` (≈95% alpha) | `theme.background.mix(95.0, TRANSPARENT)` | `bg` |
| `p-1.5` | `padding: 6px` all sides | `SPACING_1_5` | `bounds` |
| `shadow-[…]`, `backdrop-blur-sm` | painted, no field either side | nothing modelled | §6 |

Row 1 (`flex items-center gap-1.5`, `gap: 6px`): leading toggle → input →
close, all at `y: 7` (the root's own `p-1.5`). Row 2
(`mt-1.5 flex items-center justify-between gap-2`): the options group
(`gap-1`, 4px) on the left, the nav group (`gap-1`) on the right, both at
`y: 45` (`7 + 32 + 6`, the row-1 height plus the `mt-1.5` gap).

## 4. Values — the input control, and the split from `input.rs`'s own coupling

`search.tsx`'s `<Input className="ui-text-sm h-8 rounded-lg border-border/80
bg-background py-1 pr-8 pl-8" />`, where `className` is merged onto
`input.tsx`'s own **outer** `<span data-slot="input-control">` — its inner
`<input data-slot="input">` keeps `input.tsx`'s own, unmodified
`inputClassName` (`h-8.5 w-full … sm:h-7.5`).

| Anchor | React / Tailwind | Compiles to | gpui / `crowbar-ui` | Oracle |
|---|---|---|---|---|
| `input-control` | `h-8` (search's override; no `sm:` counterpart, nothing to conflict with) | `246×32` | `INPUT_CONTROL_HEIGHT` | `bounds.h` = 32 |
| `input-control` | `border-border/80` (flat replace of `input.tsx`'s own `border-input`, same group same modifier) | `#ffffff0c` | `theme.border.mix(80.0, TRANSPARENT)` | `border.color` |
| `input-control` | `bg-background` (repeats `input.tsx`'s own class — a no-op merge) | `#ffffff07` in dark | `dark:bg-input/32` — `theme.input.mix(32.0, TRANSPARENT)`, the *same* expression `button::Variant::Outline`'s background already uses for the identical Tailwind rule | `bg` |
| `input` (the field) | `input.tsx`'s own `Size::Default` at `Sm` | `180×30` | untouched — this surface never reaches `input.rs`'s `Size` enum | `bounds.h` = 30 |

**Why this could not go through `input::Input::render`.** `input.rs`'s own
`Size` enum couples the control's and the field's height together (they are
numerically equal on every call site `input.md`'s own reference covers —
that primitive has never needed to decouple them). `search.tsx` is the first
call site that *does* decouple them, and there is no parameter on `Input`
that reproduces it — so `crowbar_ui::components::search::input_control`
builds both boxes by hand, reusing `input::ID_CONTROL`/`input::ID_FIELD`
verbatim (the same ids `input.tsx` already hard-codes onto *any* `<Input>`,
this call site included) so a reader comparing this surface's reference
against `input.md`'s own compares the same two anchors on purpose.

## 5. Values — the toggle buttons

`searchToggleButtonVariants({ active })`: `flex size-6 items-center
justify-center rounded-lg border border-transparent` at rest, swapping to
`border-border/70 bg-muted text-foreground` when `active`.

| State | React / Tailwind | gpui / `crowbar-ui` | Oracle |
|---|---|---|---|
| resting | `text-muted-foreground` | `theme.muted_foreground` | `fg` on the wrapped run (only the `preserve-case` cell paints one) |
| `active` | `border-border/70 bg-muted text-foreground` | `theme.border.mix(70.0, TRANSPARENT)`, `theme.muted`, `theme.foreground` | **not driven live** — `search-toggle-icons.md` §6 already measured the one field this state moves (the wrapped glyph/run's colour), and this port reuses that finding rather than re-deriving it |

The glyph/run each toggle wraps is `crowbar_ui::components::
search_toggle_icons::SearchToggleIcon::render`, called unmodified with this
surface's own `breakpoint`/`active` — the already-verified surface, reused
rather than duplicated.

**Anchor ids are derived from `option.id`, not hard-coded.** `search.tsx` has
no built-in notion of "case" or "regex" — `options` is a generic
`SearchToggleOption[]`, so the React edit is
`data-oracle-id={\`search-toggle-${option.id}\`}`. `find-bar.tsx`'s and
`terminal-search.tsx`'s shared `option.id`s are `"case-sensitive"`,
`"whole-word"`, `"regex"`, `"preserve-case"`, giving
`search-toggle-case-sensitive`/`search-toggle-whole-word`/
`search-toggle-regex`/`search-toggle-preserve-case` — the four ids
`crowbar_ui::components::search::{ID_TOGGLE_CASE, ID_TOGGLE_WHOLE_WORD,
ID_TOGGLE_REGEX, ID_TOGGLE_PRESERVE_CASE}` carry.

## 6. Values — the nav chevrons, and the disabled-opacity trap

`searchIconButtonVariants({ disabled: !canNavigate })` — but the reachable
cell's nav buttons (`onPrevious`/`onNext`, both wired with the real HTML
`disabled={!canNavigate}` attribute) measured **`opacity: 0.64`**, not the
`0.5` `searchIconButtonVariants`' own `disabled` arm (`opacity-50`, a plain
unconditional class) would suggest.

**Why**: `button.tsx`'s own base class list carries `disabled:opacity-64` —
an attribute-selector *pseudo-class* rule, one specificity tier above a
plain class, so it wins the cascade regardless of source order once the real
`disabled` attribute is present. `search`'s own `opacity-50` never has a
chance to paint on these two cells. The port therefore reaches for
`button::DISABLED_OPACITY` (0.64) rather than defining its own 0.5 — the
number actually rendered, confirmed by reading `getComputedStyle(prevBtn)
.opacity` on the live, disabled cell.

`SearchPopover::can_navigate` moves this and only this — the box does not
move (`24×32` either way), matching the live measurement.

## 7. Values — `SearchReplaceRow`, and the finding that overturns `button.rs`

Captured separately, `/tmp/p3-ref-search-replace-row.json` (see §9 for why).

| Anchor | React / Tailwind | Measured | gpui / `crowbar-ui` |
|---|---|---|---|
| `search-replace-row` | `flex items-center gap-1.5 border-border/60 border-t pt-1.5` | `306×39` | `SearchReplaceRow::render`'s own shell |
| `search-replace-icon` | `flex h-8 w-8 … rounded-lg border border-border/70 bg-background text-muted-foreground` | `32×32`, `bg #1f1f1eff` (plain `theme.background`, **no** opacity mix — unlike the popover root's own `/95`), `border #ffffff0b` | `theme.background`, `theme.border.mix(70.0, TRANSPARENT)` |
| `input-control`/`input` (this row's own `Input`) | narrower (`flex-1`, shares the row with two buttons) | `140.81×32` / `138.81×30` | `input_control(theme, anchors, None)`, the same function §4 documents |
| `search-replace-confirm` ("Replace") | `searchActionButtonVariants`: `h-8 px-2.5 …`, **content-sized** | `75.81×32`, `text_width 53.8` | `REPLACE_ACTION_HEIGHT`/`_PADDING_X`/`_BORDER_WIDTH`, `content_sized()` |
| `search-replace-all` ("All") | same variant | `39.38×32`, `text_width 17.37` | same |

**`button.rs`'s module docs state `CONTENT_SIZED = []` and give the reason:
"the five non-icon sizes author no width... no live call site renders a
Button with a label."** `SearchReplaceRow`'s two action buttons are exactly
that shape — `<Button variant="ghost" compact>Replace</Button>` merged with
a `px-2.5`, no-`w-*` variant — and they **are** live: reachable by expanding
the editor find bar's replace row (the chevron `SearchReplaceToggle`
renders). Both are genuinely content-sized (v1.5): each button's box is its
own run's shaped advance plus this button family's own
`2×PADDING_X + 2×BORDER_WIDTH` chrome (`22.01`px on the measured cell,
`2×10 + 2×1 = 22` accounting for it), never a fixed width.

**`button.rs` is not wrong about its own captured cells** — no live `Button`
call site anywhere else in `web/src/` passes a label, which is what its own
finding measured. Its *universal* phrasing ("no live call site renders a
Button with a label") is what this item's own measurement contradicts, and
the contradiction is recorded here rather than silently worked around or
used to justify routing these two buttons through `Button::render` (which
could not paint their strings anyway — see §1).

## 8. Declarations

* `CONTENT_SIZED = [search-replace-confirm, search-replace-all]`. Both
  labelled buttons in `SearchReplaceRow` — see §7. Neither `SearchPopover`'s
  nor `SearchReplaceToggle`'s own anchors qualify: every one of them is the
  fixed `size-6`-derived box §2 measures, never a run's own width.
* `LINE_SIZED = []`. Every button on this surface authors its own height —
  `icon_button_extent` or `search.tsx`'s own `h-8` — the same shape
  `button::LINE_SIZED` is empty for, and for the identical reason.

## 9. Reachability, and the capture that could not be taken in one walk

Both references captured live 2026-08-02 against the running dev-desktop
app (`oracle-fixture/home` workspace), via the editor's find bar
(breadcrumb's *Find in file* magnifying-glass button; `.click()` reached
React with `document.hasFocus()` false throughout, per the environment
notes). `data-oracle-*` was injected with `setAttribute` immediately before
each capture (the dev server serves the **shared** worktree, so this
branch's `search.tsx` edits are not live there) and removed immediately
after; the file tree and the find bar were both closed again afterward to
leave the shared session as found.

* **`/tmp/p3-ref-search.json`** — root `search-popover`, replace collapsed
  (find-bar.tsx's default `isReplaceVisible: false`): the leading toggle,
  the input, close, the three always-live option toggles, both nav chevrons
  (disabled — the query is empty).
* **`/tmp/p3-ref-search-replace-row.json`** — root `search-replace-row`,
  captured with the replace row expanded (clicking `SearchReplaceToggle`):
  the swap icon, the row's own `Input`, and both labelled action buttons.

**Why two captures and not one.** Expanding the replace row mounts a
*second* `<Input>` while `SearchPopover`'s own is still mounted, and
`web/src/components/ui/input.tsx` **hard-codes** `data-oracle-id`
(`"input-control"`/`"input"`, literal JSX attributes) with **no override
prop** — unlike `button.tsx`'s `'data-oracle-id': 'button', ...props`, which
a call site *can* shadow. `extractSnapshot({ root: 'search-popover' })`
walked with both `Input`s mounted would therefore record the id
`"input-control"` twice under one root. Rooting the second capture at
`search-replace-row` (a sibling subtree of `SearchPopover`'s own row 1, not
its ancestor) sidesteps the collision entirely, because
`rootEl.querySelectorAll('[data-oracle-id]')` only walks descendants of the
chosen root. **This is a genuine, load-bearing limitation of the
already-verified `input` primitive**, discovered by this port and recorded
rather than smoothed over — a future item giving `input.tsx`'s two
`data-oracle-id`s an override prop (`button.tsx`'s own pattern) would let a
single combined capture replace these two.

**Reachable, both importers**: the three always-live toggles
(`case-sensitive`/`whole-word`/`regex`) and the shell/input/close/nav
chevrons. **Reachable, `find-bar.tsx` only**: `SearchReplaceToggle`, the
fourth (`preserve-case`) toggle, and `SearchReplaceRow` — `terminal-search.tsx`
never renders a `leadingControl` or a `secondaryRow`, matching
`search-toggle-icons.md` §5's own reachability table for the same four
toggles.

**Not reachable, real but unmodelled**, for reasons already established
elsewhere in this tree:

* `matchLabel` — no live cell had a non-empty query, `hover`'s own reason
  (`CGPreflightPostEventAccess()` false; nothing in this session could type
  into the field and read the resulting `N/M` span).
* The clear (`×`) button inside the input — renders only when `value` is
  truthy, and the reachable cells all had an empty query. Not anchored on
  the React side either, for the same "do not invent a thing to verify"
  reason Part 2 of this item's brief names.
* A toggle's `active` state on the *box* — not driven live this pass;
  `search-toggle-icons.md` §6 already measured the one field `active` moves
  on the glyph/run each toggle wraps (`fg`, and only on the `preserve-case`
  cell), so this port reuses that finding rather than re-deriving it against
  a state nobody drove again.

## 10. Identity evidence — per captured element, before any attribute was injected

Read live off the running app, `getBoundingClientRect`/`className`/
`getComputedStyle`, before `data-oracle-*` was set on anything:

```
search-popover root:
  className: "w-[320px] rounded-xl border border-border/70 bg-background/95 p-1.5
              shadow-[0_16px_36px_-28px_rgba(0,0,0,0.55)] backdrop-blur-sm"
  data-slot: null
  rect: 320×84 at (1133, 82), logical px, 1714px viewport, dark
  children: 2 (row 1, row 2) collapsed / 3 (+ secondary row) expanded

Row 1 children (real primitives): leading-toggle <button data-slot="button">,
  Input's <span data-slot="input-control"> wrapping <input data-slot="input">,
  close <button data-slot="button"> — 3 real interactive primitives.

Row 2: options group (3 <button data-slot="button"> at rest, 4 with replace
  expanded, each wrapping a phosphor <svg> or the search-toggle-icons "Aa"
  <span>), nav group (2 <button data-slot="button">) — 5–6 real primitives.

search-replace-row (expanded only):
  className: "flex items-center gap-1.5 border-border/60 border-t pt-1.5"
  children: icon <span> (no data-slot), Input's own two elements, two
  <button data-slot="button"> reading "Replace"/"All" — 4 real primitives
  plus the icon span (5 total).
```

Total real primitives on the reachable, fully-expanded cell: **1** root +
**3** row-1 children + **6** row-2 children (4 toggles + 2 nav) + **5**
replace-row children = **15** anchorable elements, none constructed or
injected — every one reached by opening the real find bar and reading the
live DOM. No element in this item was fabricated: every capture in §9 reads
an element that `.click()` and `setAttribute` reached on the running app,
never a stand-in built to match an expected geometry.

## 11. What is not modelled, stated rather than left implicit

* `extraActions` — a `ReactNode` prop no live call site passes.
* `inputName` — an accessibility attribute, not a visual property.
* `matchTone` (`'warning'` colouring the match-count span amber) — the span
  itself is unreachable (§9), so its tone is doubly so.
* Hover and focus-visible rules on every button (`hover:border-border/70
  hover:bg-muted hover:text-foreground`, `focus-visible:ring-2`) —
  unmodelled for `button`'s own reason: no reference (synthetic pointer
  events are denied on this project's machines), and a focus ring is two
  box-shadows `ANCHORS.md` §6 has no field for.

## 12. P3.37 — `search-replace-row` becomes a second, standalone surface

This item's own §9 already named the problem `/tmp/p3-ref-search-replace-row.json`
turned out to have: its `root` is `search-replace-row`, but nothing registered
under that root existed to compare it against — `surfaces/search.rs`'s own
module docs (added alongside this file) said as much and called the missing
piece "a second, scoped capture … not a flag on this one," without building
it. P3.37 built it: `surfaces/search_replace_row.rs`, root
`search::ID_REPLACE_ROW`, reusing `SearchReplaceRow` unmodified via a new
`render_root` method (`AnchorSink::root`, the sibling of the existing
`render`'s `AnchorSink::boxed`) rather than a second implementation.

**Why folding the row's anchors into `search`'s own declared scope was not
available**, checked both directions:

* On the DOM side, expanding the replace row mounts a *second* `<Input>`
  while `SearchPopover`'s own is still mounted, and `input.tsx` hard-codes
  `data-oracle-id="input-control"`/`"input"` with no override prop — a
  document spanning both carries the id twice, which `ANCHORS.md` v1.8
  refuses outright regardless of what any scope declares.
* On the driver side, `AnchorRegistry::record` **replaces** rather than
  refuses a repeated id, so `--surface search --replace` does not crash — but
  its recorded `"input-control"` silently becomes whichever `Input`
  prepainted last (this row's own, narrower one), and `Snapshot::build`
  copies *every* recorded anchor into a snapshot regardless of which id
  `root` names — it re-origins, it does not filter by subtree. So even asking
  that same run for `root: "search-replace-row"` would still carry
  `search-popover`, `search-close` and the rest, not the six-anchor shape the
  reference has.

Only a render pass that paints **nothing but** `SearchReplaceRow` produces a
registry `Snapshot::build` can turn into the reference's own shape, which is
what the new surface is for. Verified directly:
`row_layout/search_replace_row.rs`'s
`the_registry_holds_only_this_rows_own_six_anchors` measures the surface and
asserts the six ids and nothing else — in particular not `search-popover` —
which is the property this whole section argues for, not merely asserted.

**The state axis.** An early draft carried `can_replace`/`can_replace_all` as
two CLI flags and declared all six `StateFlag`s unmodelled, which
`surface::tests::no_surface_declares_its_entire_state_axis_unmodelled`
correctly refused: a surface with no real axis at all is one whose whole
matrix cannot fail. The fix reads `StateFlag::Empty` — "the search that feeds
this row found nothing, so neither action has anything to do" — the same
fact `search`'s own default cell already encodes as `can_navigate: false` for
an empty query, and the same flag `sidebar_header`/`scroll_area`/
`sidebar_empty` each fall back to for their own domain's "nothing here" when
no other flag fits.

## 13. P3.44 — a side-by-side parity run against the live app, and three fixes

Driven independently of this file's own captures: the requester ran both
apps side by side and diffed the anchored geometry directly, rather than
re-deriving it from `/tmp/p3-ref-search.json`/`/tmp/p3-ref-search-replace-row
.json`. Three defects came back, all on the native side; the reference
needed no change for two of them and had already been fixed for the third.

**The `input` anchor's box was on the wrong offset from `input-control`, in
both surfaces, for the same wrong reason.** §4 and §7's tables above give
the two boxes' final sizes but never state what *positions* the inner field
inside the outer control — that gap is where the defect lived.
`crowbar_ui::components::search::input_control`'s first draft insetted the
field by a flat `px(11.0)` on every side, on both call sites, centred
vertically with `.items_center()`. Measured live, the two surfaces disagree:
`search`'s own popover insets its main input by **33px** each side
horizontally and **5px** from the top; `SearchReplaceRow`'s insets by only
**1px** horizontally and the same **5px** from the top. The mechanism,
worked out from `search.tsx` rather than guessed at: both of `Input`'s
`className` overrides land on the **outer** `<span data-slot=
"input-control">`, never the inner field (§4's own finding, restated here
because it is what makes the fix possible) — `SearchPopover`'s Input carries
`pl-8 pr-8 py-1`, `SearchReplaceRow`'s carries only `py-1`. The control's own
`border` (1px) is constant on both; `pl-8`/`pr-8` (`--spacing(8)` = 32px)
is what turns that 1px into 33 wherever it is present, and its absence is
what leaves `SearchReplaceRow`'s inset at the bare 1px. `py-1`
(`--spacing(1)` = 4px) is on *both* overrides, so the vertical inset (1px
border + 4px padding = 5px) does not vary the way the horizontal one does —
only the port's use of `.items_center()` (which centres the field's fixed
30px height inside the control's 32px, landing on 1px) rather than
`.items_start()` plus an authored `padding-top` was wrong. Fixed by giving
`input_control` a `padding_x: Pixels` parameter
(`INPUT_PADDING_X_ICONED` = 32, `INPUT_PADDING_X_PLAIN` = 0) and switching
its vertical alignment to `.items_start()` with an explicit `.pt()`/`.pb()`
of 4px. Pinned as the *relationship* (field origin minus control origin),
not the absolute coordinates, in `row_layout/search.rs`'s
`the_input_field_insets_from_its_control_by_the_icon_padding` and
`row_layout/search_replace_row.rs`'s
`the_input_field_insets_from_its_control_by_only_the_border`.

**The two labelled replace buttons' `font.line_height` read `22.5`, not the
reference's `20.0`.** §7's table above gives their box and border but never
their type step. `action_button`'s first draft called `.text_size(theme
.ui_text_base.value())` and never called `.line_height(…)` at all, so gpui
fell back to its own default rather than anything either `search.tsx` or
`button.tsx` compiles to. The two buttons are real `<Button>`s, and
`buttonVariants`' own base class list (`button-variants.ts`) carries
Tailwind's stock `sm:text-sm` unconditionally — a *different* class from
`searchActionButtonVariants`' own `ui-text-sm` (a custom utility,
`web/src/index.css`, that sets `font-size` and nothing else), so
tailwind-merge does not treat the two as conflicting and both compile in.
At the `sm` breakpoint `sm:text-sm`'s media-scoped rule wins the cascade for
`font-size` over `ui-text-sm`'s plain one — the identical mechanism §2 above
already documents for these same buttons' *height* — and, because
`ui-text-sm` never set a line-height at all, `sm:text-sm`'s own paired one
(`calc(1.25rem / 0.875rem)`) applies unopposed: `14 × 1.42857 = 20.0`, the
reference's number. `button::Size::Default::type_step` already carries
exactly that `(size, line_height)` pair — every ordinary `Button::render`
call already reads it — so `action_button` now reads it too rather than
hand-picking half of it and leaving the other half to gpui's default. Pinned
in `row_layout/search_replace_row.rs`'s
`the_labelled_buttons_take_text_sms_line_height`.

**`search-toggle-icon` was being recorded on this surface a second time.**
§5 above documents that the three (or four) option toggles each wrap
`crowbar_ui::components::search_toggle_icons::SearchToggleIcon`, reused
unmodified — what it does not say is that `SearchToggleIcon::render` opts
its one anchor id into *whatever* `AnchorSink` it is handed, and
`toggle_button` used to hand it this surface's own. Every toggle on a cell
therefore recorded `search-toggle-icon` into `search`'s own registry, and
because `AnchorRegistry::record` replaces rather than refuses a repeated id
(untouched here — a different worker's branch,
`native/p3.42-recorder-strictness`, is the one making it refuse), a
`--surface search` snapshot silently ended up with exactly one such record,
overwritten down to whichever toggle painted last. The reference carries
**none**: §9's own "Reachability" account above already explains why
`search-toggle-icons` is `search`'s sibling surface and not its own
anchor, and `web/src/lib/oracle/extract.ts`'s `oracleSurfaceScope` already
enforces it — `search`'s declared ten ids do not include
`search-toggle-icon`, and `oracleSelectDeclaredAnchors` drops anything found
under the root that is not declared. That half of the fix already existed
on this branch before this item; only the native side still recorded the
duplicate. Fixed by rendering the icon through `Unanchored` inside
`toggle_button` rather than this surface's own `anchors` —
`Unanchored`'s own contract guarantees the identical box, so nothing this
file's tables above give moves; only the id stops being recorded a second
time. No new id was invented for it (`search-toggle-icon-regex` and so on
would describe geometry `search-toggle-icons` already owns). Pinned in
`row_layout/search.rs`'s `the_toggle_icon_is_not_recorded_on_this_surface`.
