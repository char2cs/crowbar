# `repo-icon-popover` (P3.58)

`web/src/components/layout/repo-icon-popover.tsx` →
`crates/crowbar-ui/src/components/repo_icon_popover.rs`,
`crates/crowbar-app/src/surfaces/repo_icon_popover.rs` (the popup only),
`crates/crowbar-app/src/row_layout/repo_icon_popover.rs` (the popup, plus
the trigger, driven without a `--surface` — see §5).

**No live reference.** This item does not run the oracle or capture a
snapshot. Worse than usual for this one specifically: `avatar.rs`'s own
module docs, written before this item, already found this exact popover
**structurally uncapturable** — it lives inside a `PopoverContent`, a portal
that exists only while open, and synthetic pointer events are denied on
these machines. Every number below is read off the app's own compiled
Tailwind, never off a capture.

## 0. Two pictures, two different geometries, one component file

The trigger (always on screen, a sidebar row's own icon) and the popup
(ephemeral, opened on click) are different DOM subtrees with different
roots. `repo_icon_popover.rs` models both — `Trigger` and `PopupContent` —
because the brief asks for the whole component ported, but only the popup
gets a registered `--surface`: see §5 for why.

## 1. Values

| React / Tailwind | Compiles to | gpui / `crowbar-ui` |
|---|---|---|
| `h-5 w-5` (trigger) | 20px | `TRIGGER_SIZE` |
| `rounded-md` (trigger image/letter) | `theme.radius_md` — **not** `repo_avatar::Size::Lg`'s own `rounded-sm` | reused via `theme.radius_md` at render time |
| `px-1` (trigger letter) | 4px | `TRIGGER_LETTER_PADDING_X` |
| `text-[11px]` (trigger letter) | 11px, coincides with `repo_avatar::Size::Lg`'s own letter size but not derived from it | `TRIGGER_LETTER_TEXT` |
| `text-lg` (trigger emoji) | 18px | `TRIGGER_EMOJI_TEXT` |
| `w-64` (popup) | 256px | `POPUP_WIDTH` |
| `p-3` (popup inner) | 12px | `POPUP_PADDING` |
| `gap-3` (popup rows) | 12px | `POPUP_GAP` |
| `gap-1.5` (button row, emoji row) | 6px | `ROW_GAP` |
| `text-[10px]` (caption) | 10px | `CAPTION_TEXT` |
| `size-14 rounded-xl text-base` (preview avatar) | `avatar::CallSite::RepoIcon`'s own pre-measured extent/radius/weight | reused directly, not re-derived (see §2) |
| `size-3` (button glyphs) | 12px | `BUTTON_ICON_SIZE` |
| `h-7` (emoji input, Set button) | 28px | `EMOJI_ROW_HEIGHT` |
| `size="xs"` (Upload/Emoji/GitHub/Reset) | `button::Size::Xs`'s own extent/padding/gap/radius | reused, not re-derived |

## 2. The preview avatar reuses `avatar::CallSite::RepoIcon`, and closes part of a gap that surface's own docs flagged

`avatar.rs` already models this exact call site's `<Avatar className="size-14
rounded-xl text-base">` as `CallSite::RepoIcon` — but flags its own three
fallbacks as **not modelled**, because the third (`repo.avatarColor`) is
data a port cannot resolve into a class name. This file resolves that the
way `repo_avatar.rs` resolves the identical problem for its own letter
fallback: `PreviewAvatar::Letter::background` is a caller-supplied `Color`,
not invented. `PreviewAvatar` also splits the two real fallback pictures
`avatar.rs`'s own generic `Initials` cannot: the emoji fallback is
`text-2xl` (24px), the letter fallback is `text-sm font-bold` (`--ui-text-
base`, the `text-sm == --ui-text-base` trade `native/MAPPING.md` states).

`avatar::Avatar::render` itself is not called — it opts its own
`anchors.root(...)` in, and nesting a second root inside this surface's own
would contest which anchor `ANCHORS.md` §4 means, the `sidebar_project_
header.rs`-does-not-call-`Button::render` shape one door over.
`PreviewAvatar::render` reuses `CallSite::RepoIcon.extent()`/`.radius()`/
`.weight()` and opts the **same** ids — `avatar::ID_ROOT`/`ID_IMAGE`/
`ID_FALLBACK` — in via `.boxed()` instead, matching what the real DOM (were
it capturable) would carry, since `repo-icon-popover.tsx`'s own `<Avatar>`
passes no override.

## 3. The emoji row's `<Input>` is the shared primitive, ids and all

`input.rs`'s own `data-oracle-id="input-control"`/`"input"` are hard-coded
with no override prop (unlike `button.tsx`'s), so the emoji field really
does carry those two generic ids in the live DOM. `Input::render` is not
called for the same root-collision reason as §2; `PopupContent::emoji_row`
reproduces a simplified box and opts the two real ids in via `.boxed()`
instead. No live reference exists for this row (`showEmojiInput` starts
`false`, and this popover cannot be opened by any capture technique this
port can use regardless — see the header), so the box is not claimed to be
pixel-exact against `input.rs`'s own painted properties, only anchor-exact.

## 4. Anchoring

`repo-icon-popover.tsx` carried no `data-oracle-id` before this item. Eight
are added:

* `repo-icon-popover-trigger` — on **both** of the component's mutually
  exclusive rest states (the `repo.defaultWorking` spinner span, and
  `<PopoverTrigger>`), the same "one id, several mutually-exclusive
  pictures" shape `repo_avatar.rs`'s own leaf takes.
* `repo-icon-popover-popup` — on `<PopoverContent>`, namespaced away from
  `popover.tsx`'s own generic `"popover-popup"` default. The same "call
  site of a shared primitive" finding `detach-holder-modal-popup`/
  `repo-import-dialog-popup` (P3.51) already made: `surface.rs`'s registry
  requires a unique root per surface, and reusing the generic id would
  collide in fact.
* `repo-icon-popover-upload`/`-emoji`/`-github`/`-emoji-submit`/`-reset` —
  on the five buttons, namespaced away from `button.tsx`'s own repeated
  `"button"` default (all three of the always-visible buttons would
  otherwise share one id in the same document, an `ANCHORS.md` v1.8
  refusal).

Composed, not authored here: `repo-avatar` (image-loaded trigger case
only — see the component's own module docs for why the emoji/letter trigger
cases carry no id, matching the hand-rolled spans that never call
`RepoAvatar()`), `avatar`/`avatar-image`/`avatar-fallback` (the preview),
`workspace-branch-icon`/`flicker-spinner` (both rest states' spinner
branch), `input-control`/`input` (the emoji row).

## 5. No `oracleSurfaceScope` entry, and only one `--surface`

Every nested anchor this composition can reach is one it actually paints —
not foreign content left unpainted the way `sidebar-project-header`'s
toggle icon is — so no scope entry is needed. `input-control`/`input` are
the one **generic**, unnamespaced id reached here; they stay undeclared
because at most one `<Input>` is ever mounted in this popup at once, so no
capture this surface can produce carries it twice.

Only the popup is a registered `--surface repo-icon-popover` (root
`repo-icon-popover-popup`) — the same choice `popover`/`detach-holder-modal`
make: the floated content is the surface, a call site's own trigger chrome
is not separately registered anywhere in this tree. `Trigger`'s own two rest
states are real, tested geometry too, driven directly through the shared
`row_layout` harness instead (`row_layout/repo_icon_popover.rs`), the
`sidebar_tab_bar.rs` shape — a second root would need a second surface this
file has no other use for.

## 6. Declarations

`CONTENT_SIZED = []`. `LINE_SIZED = []` — every box is authored or an
empty/reused glyph box.

## 7. The state axis

Every one of the six §8.3 flags is unmodelled — `<PopoverContent>` carries
no `hover:`/`focus:`/`data-active` rule of its own; every interactive class
list lives on the buttons inside it, `button`'s own surface's business.
`--preview`/`--emoji`/`--reset` are this surface's own axis instead.
`Params::no_state_axis()` returns `true`.

## 8. `row_layout` coverage

* the default cell carries every unconditional anchor and neither optional
  row
* `--emoji`/`--reset` each add their own button independently, and `--emoji`
  also mounts `input-control`/`input`
* `--preview image` swaps `avatar-fallback` for `avatar-image`, never both
  at once
* the preview avatar is a real 56px square, read through a layout
* the popup's own width is always 256px, whatever `--width` the cell drives
* the trigger carries `repo-avatar` only on the loaded-image picture
* `working: true` replaces the whole trigger with the spinner regardless of
  `picture`
* the trigger is always a 20px square

## 9. Reachability

The repo section's own row (`repo-section.tsx` → `ide-shell.tsx` →
`workspace-tree`), one per repo — confirmed live in
`native/mapping/liveness-audit.md`.

---

## 6. VERDICT: FAIL — 36 deltas over 6 anchors (2026-08-03, my own run)

Drive: `--surface repo-icon-popover --width 256 --viewport-width 1714 --theme
dark --content normal --preview letter`, against the live popover opened by
clicking `repo-icon-popover-trigger`. **The cell is right** — I checked the
live popup's own text content (`IconDUploadEmojiGitHub`) against the fixture's
before reading a single delta, so neither `--emoji` nor `--reset` belongs in
this cell and the deltas are not another wrong-cell run.

### The one root cause behind 17 of the geometry deltas

The popup's own box arithmetic resolves exactly on both sides, which is what
makes this diagnosable rather than a pile of numbers:

| | React | port |
|---|---|---|
| border | **1** | 0 |
| `popover-viewport` padding | **16** | **not modelled at all** |
| inner `p-3` | 12 | 12 |
| caption line box | 15 | 15 |
| `gap-3` | 12 | 12 |
| **= avatar `y`** | **56** ✓ | **40** ✓ |
| **popup `h`** | **177** ✓ | **144** ✓ |

Both columns are internally consistent, so nothing here is a measurement
artefact. The port is missing exactly two things — the popup's **1px border
and 10px radius**, and the **`popover-viewport` element and its 16px
padding**. `1 + 16 = 17` accounts for the 16px `y` shift on every child, and
`2 + 32 = 34` accounts for the 33px height delta.

`popover-viewport` is also absent as an *anchor*, which is the single anchor-
presence delta. The port hand-rolls this popup instead of composing the
`popover` primitive the React call site actually uses.

### The remaining deltas, by kind

- **15 field-presence**: the three action buttons emit no `text`, `fg`,
  `text_width`, `font` or `clipped`. The port paints their labels as unanchored
  children, so the contract can see the boxes but not the text.
- **3 button widths** (73.5 vs 69.63/59.77/69.56): the port gives all three
  buttons one shared width; React content-sizes each. `Emoji` is the tell —
  59.77 against the port's flat 73.0.
- **`avatar-fallback`**: `line_height` 22.5 vs 20, `text_width` 8.946 vs 10.64.
  Font metrics on the letter, not layout.
- **`avatar-fallback.text: "R", expected "D"`** — **a fixture gap, not a port
  defect.** The native fixture hard-codes `R`; the live repo is `demo`. There
  is no `--letter`/`--name` flag to drive it, so this delta cannot be closed by
  driving. It needs a flag.

### Corroboration worth keeping

The caption's own line box measures **15px at `text-[10px]`** —
`10 × 1.5`, Tailwind's preflight default, on an element with no paired
line-height utility. That is P3.60's `row_base` finding reproduced
independently on a different component at a different font size, which is
about as good as confirmation gets for that ratio.

---

## FIXED (2026-08-03, follow-up item) — the fixture gap only

`--surface repo-icon-popover` now takes `--letter <text>`, read only with
`--preview letter`, overriding `PreviewAvatar::Letter`'s own label (it was
hard-coded to `"R"`) — see
`crates/crowbar-app/src/surfaces/repo_icon_popover.rs`. `avatar-fallback.text`
above can now be driven to match a live repo's own initial.

Every other delta this VERDICT records (the 17-delta box-arithmetic root
cause, `popover-viewport` missing as a composed primitive and an anchor, the
15 field-presence deltas on the action buttons, the 3 button-width deltas,
and the `avatar-fallback` font-metric deltas) is **untouched** — out of
scope for this follow-up item, which addressed only the one hard-coded
fixture string named in its brief.

> **Superseded in scope by P3.63, merged the same day.** The paragraph
> above was accurate when written — that follow-up item touched only the
> fixture string. §7 below is the item that closed the rest, so "untouched"
> now describes a window that has closed, not the current state.

## 7. FIXED (P3.63)

All of §6 except the one declared out of scope. `crowbar_ui::components::
repo_icon_popover::PopupContent::render`'s own doc comment carries the
account in full; this section is the index into it.

**The one root cause (17 deltas).** `PopupContent::render` now composes
`popover`'s own chrome instead of hand-rolling a plain box: the popup gets a
real border (`popover::BORDER_WIDTH`, reused) and radius (`theme.radius_lg`,
the same token `popover::Variant::Default::radius` reads), and a new
`popover-viewport` box — the primitive's own **generic**, unnamespaced id,
reused rather than re-typed, the same move `PopupContent::emoji_row` already
makes of `input.rs`'s `input-control`/`input` — carries `popover::
VIEWPORT_PADDING` (16px) on every side. This is composition, not a call to
`popover::Popover::render`: that constructor opens through `gpui_component::
Popover`'s deferred, anchored placement, which this call site — driven
directly, with no live trigger — does not want, so the two boxes `popover::
Popover::popup`/`::viewport` build are reproduced here directly instead, at
this surface's own root id. Verified by `row_layout::repo_icon_popover::
the_popup_composes_popovers_border_radius_and_viewport`: the popup now
measures 256×177 and the viewport 254×175, one border in on both axes — the
exact numbers this section's own verdict targeted.

**A fourth line-height instance the verdict's own arithmetic did not name.**
Fixing the border and viewport alone left the preview avatar's own `y` at
`57`, one pixel short of the `56` target. The caption (`text-[10px]`, no
paired `leading-*` utility) turned out to have the *same* unset-leaf bug as
`avatar-fallback`: no explicit line height, so it fell back to gpui's own
golden-ratio default (`~1.618034`) and measured 16px tall instead of the
15px this section's own "Corroboration" paragraph above states as the
*reference's* number — a check that was never actually run against the
*port's* own caption before this item. `PopupContent::CAPTION_LINE_HEIGHT`
(`1.5`, the same ratio, derived independently for this leaf) closes it.
Verified by `row_layout::repo_icon_popover::
the_preview_avatar_sits_56px_below_the_popups_own_top`.

**15 field-presence.** `ActionButton::render` now paints its label through
`AnchorSink::boxed_text` instead of an unanchored `.child(label)`, so `text`,
`font`, `text_width` and `clipped` all ride on the same anchor as the box —
the same primitive the caption and the preview fallback already used.
Verified by `row_layout::repo_icon_popover::
the_three_action_buttons_carry_their_own_text`.

**3 button widths.** Not a CSS-authoring bug — `repo-icon-popover.tsx`'s
buttons and the port's both say `flex-1`. Measured directly: this layout
engine grows a `flex: 1 1 0%` item by an equal share **without** first
clamping it to its own min-content, the "automatic minimum size" step a
browser applies before it distributes leftover space (confirmed by swapping
in `.flex_none()` on the same cell and watching the three widths diverge by
label alone: 60/52/60). `.flex_auto()` (`flex: 1 1 auto`) reaches the
browser's outcome without depending on a clamp this engine does not apply,
because every label here is a single non-wrapping line, so min-content and
max-content are the same width. Not byte-exact against the reference
(measured after the fix, this surface's own available width: Upload 64.5,
Emoji 57, GitHub 64.5, against the reference's 69.63/59.77/69.56) — content-
sized and correctly *ordered*, which `row_layout::repo_icon_popover::
the_three_action_buttons_size_to_their_own_label` checks directionally
rather than to the pixel, since this surface has no live reference to check
an exact width against (§0) and `.flex_auto()` is itself a **substitution**
for a clamp this engine does not implement, not a re-derivation of the
browser's own arithmetic.

**`avatar-fallback`'s `line_height`.** Fixed — `PreviewAvatar::
LETTER_LINE_HEIGHT` (`1.25/0.875`, `text-sm`'s own ratio, the same one a
dozen other `text-sm` leaves in this crate already carry) replaces the same
unset-leaf golden-ratio default the caption had, landing on `20px` against
the previous `22.5px`. Verified by `row_layout::repo_icon_popover::
avatar_fallback_line_height_is_text_sm_not_the_golden_ratio_default`.

**`avatar-fallback`'s `text_width` — investigated, not fixed, and not a
separate bug.** With the line-height fix in and the font family set
explicitly (`ui_sans_font`, matching every other leaf in this file), the
letter's own `text_width` measures `8.4px` for `"R"` at 14px bold
`CalSansUI` — font-size, weight and family all already correct, so nothing
about *this* box's own styling is wrong. `8.4` against the verdict's `10.64`
is not a residual bug; it is downstream of the fixture-flag gap §6 already
names for the neighbouring `text` field on the same anchor (`"R"` in the
fixture vs `"D"` live) — two different glyphs have two different advance
widths at an identical font, and that gap has no `--letter`/`--name` flag to
close it by driving. Left alone, as §6 already scoped.

**Not touched:** the emoji preview's own line height
(`PreviewAvatar::EMOJI_LINE_HEIGHT`, `text-2xl`'s `2/1.5`) is the same
unset-leaf bug and was fixed alongside the letter fallback's, on the same
evidence, even though `--preview emoji` has no live cell to verify it
against (§0) — preventive, not measured, and named as such in the code.

## 8. RE-VERDICT after P3.63 — FAIL 15/7, root cause closed

My own run, same cell as §6 (`--letter D` now available, so `avatar-fallback.text`
matches and is no longer a delta).

**36 → 15.** Everything §6 identified as the one root cause is gone: the popup
measures **256×177**, `popover-viewport` is present at **254×175** (one border
in on both axes), the avatar is back at **y=56**, and every position delta with
it. §6's arithmetic table now resolves identically on both sides.

All 15 survivors sit on the three action buttons:

```
{upload,github,emoji}.bounds.w:        65.0 / 65.0 / 56.0,  expected 69.63 / 69.56 / 59.77
{upload,emoji,github}.border.w:        0.0,   expected 1.0   (exact)
{upload,emoji,github}.font.weight:     400,   expected 500   (exact)
{upload,emoji,github}.font.line_height: 19.5, expected 16.0  (Δ +3.5)
```

**None of these is a regression.** Before P3.63 those anchors emitted no `text`,
`fg`, `text_width`, `font` or `clipped` at all — they were 15 *field-presence*
deltas. Painting the labels through `boxed_text` made the fields comparable, and
that is what exposed the values. **This is the second time today that fixing a
visibility problem surfaced a real defect** (the first: `workspace-tree-item`'s
`-add-child` border, invisible while the anchor was undeclared). Worth stating as
a rule: a field that is not compared cannot be wrong, and the count of
*comparable* fields is a better progress measure than the count of deltas.

`font.line_height` **19.5 against 16.0** is almost certainly the transfer this
port keeps re-learning. The reference reports `font.size: 12` — that is
`text-xs`, a **named** Tailwind step, and named steps ship a *paired*
line-height (`12px/16px`, ratio 1.333). `1.5` is what an **arbitrary**
`text-[Npx]` inherits when nothing overrides. The caption fix at 10px→15px was
right for exactly the reason these buttons are wrong.

Returned to the worker with all four, the `text-xs` pairing spelled out, and
P3.64's `Styled::font`-resets-weight finding flagged as the likely cause of the
400.

## 9. FIXED — the four §8 fields, all reused from `button.rs`

`ActionButton::render`'s own doc comment (`crowbar_ui::components::
repo_icon_popover`) carries the account in full; this section is the index.

**`border.w` and `font.weight` — exact fixes, both reused rather than
re-derived.** `button::BORDER_WIDTH` (1px) and `button::Variant::border` on
`Variant::Ghost` (`Color::TRANSPARENT`) give the button its real, always-1px
border; `FontWeight::MEDIUM`, `button.tsx`'s own unconditional `font-medium`,
gives it the right weight. **Not** the `Styled::font`-resets-weight footgun
P3.64 found and this round's brief flagged as the likely cause: checked
directly, this box never called `.font(…)` at all, so there was nothing to
overwrite — the weight was simply never set. The stray `.font(ui_sans_font
(theme))` P3.63 had added here (defensive, not load-bearing — the family
already reached this leaf by ordinary inheritance, measured correctly before
this fix too) is dropped, one keystroke from being that exact footgun on a
future edit.

**`font.line_height` 19.5 → 16 — the brief's own read confirmed.** `button::
Size::Xs.type_step(theme, BP)` already carries `text-xs`'s paired ratio
(`button::LINE_HEIGHT_XS`, `1/0.75`) in its own `line_height` field; this box
computed `step` for its font *size* and never read `step.line_height` at
all, so it fell back to gpui's golden-ratio default the same way the caption
and `avatar-fallback` did before their own P3.63 fixes — confirmed by
mutation: dropping the line-height call reproduces `19.5px` exactly.

**`bounds.w` — closed as far as arithmetic can close it, not byte-exact.**
Two sub-fixes, both `button::Size::Xs`'s own already-verified numbers:

* `button::Size::padding_x()` (7px each side, `px-[calc(--spacing(2)-1px)]`)
  — this box had never called it, carrying no horizontal padding at all.
* the icon's own `[&_svg]:-mx-0.5`, resolved into the glyph's box rather than
  declared as a margin — `button.rs`'s own module docs call this **"the
  largest finding in this port"**: a negative inline margin on an in-flow,
  content-sized flex item breaks taffy's main-size resolution outright, and
  `button::Button::glyph`'s established fix is to shrink the box and apply
  no margin at all. This surface's own icon is call-site-sized (`size-3`,
  not `button.rs`'s own computed size), so the same arithmetic is applied by
  hand: `BUTTON_ICON_SIZE + button::ICON_MARGIN_X * 2.0` = 8px in-flow,
  height unchanged.

Measured after both: `72 / 64 / 72`, against the reference's `69.63 / 59.77
/ 69.56` — much closer than the pre-fix `65.0 / 65.0 / 56.0` (§8), but not
exact, and **that residual is not chased further, on evidence rather than a
shrug**: `row_layout`'s own harness never shapes a real glyph.
`#[gpui::test]`'s `TestPlatform` hardcodes a `NoopTextSystem`
(`vendor/gpui/src/platform/test/platform.rs`), so every text width this
port's own tests can measure is against a synthetic stand-in font, never the
live app's real `CalSansUI` — the exact reason `row_layout::badge`'s own
default-cell test already declines to assert its width against the
reference's pixel value ("the shaped advance … and the two engines shape
independently"). Only the live binary (`--features driver`, which *does*
load real `CalSansUI` through the same `MacTextSystem` `main.rs` registers
it into) can verify the shaped term, and this item does not run it. What
this fix closes is everything upstream of the glyph shape — border, padding,
icon margin, gap — all now `button.rs`'s own numbers rather than re-derived.

**A flex-mode footnote, corrected mid-item.** A first pass swapped
`.flex_1()` for `.flex_auto()` to route around the missing automatic-
minimum-size clamp (§8's `border.w`/`padding` finding). Once the button's
real chrome is in place, the three buttons' own combined footprint (220px
with gaps) already exceeds the row's 198px share of the popup, so neither
flex mode has anything left to distribute — measured identical under both,
`72/64/72`. The literal, faithful `.flex_1()` (matching `repo-icon-
popover.tsx` exactly) is what shipped; `.flex_auto()` was a workaround the
border/padding fix made unnecessary.

Four `row_layout` tests added for this round (border+colour, weight,
line-height, and a refreshed width-ordering test whose real mutation is now
stripping the border/padding back off, which reopens the flat-split bug),
each with its mutation actually run and the real failure output kept in the
doc comment.
