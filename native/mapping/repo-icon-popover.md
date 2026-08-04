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
