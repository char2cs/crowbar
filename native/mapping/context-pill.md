# `context-pill` (P3.58)

`web/src/components/layout/context-pill.tsx` →
`crates/crowbar-ui/src/components/context_pill.rs`,
`crates/crowbar-app/src/surfaces/context_pill.rs`,
`crates/crowbar-app/src/row_layout/context_pill.rs`.

**A live reference exists — not captured by this item, but landed against
it — and it now reconciles exactly.** The item that ported this surface
drove no oracle and captured no snapshot, and every number in the first
version of this file was read off the app's own compiled Tailwind alone. A
parity run taken afterward, against `--surface context-pill --kind home
--width 344`, found the trigger 1px taller than the reference and missing
a real border. Three real defects are fixed (§2): a missing 1px transparent
border, a wrong line-height on the label stack's large line, and — the one
that took two more passes to land — the small line rendering at its own
*computed `line-height` property* rather than its own *rendered box
height*, two different numbers on this particular line that `getComputedStyle`
and `getBoundingClientRect` each answer differently. `row_layout::
context_pill::the_live_parity_cell_matches_the_reference_within_tolerance`
reproduces the reference exactly: trigger `47px`, root `51px`, `border.w`
`1`, `border.color.a` `0`.

## 0. What this file is, and what it is not

`context-pill.tsx` is `ContextPill()`: the sidebar's "you are here" pill,
always mounted, rendering one of three pictures (`kind: 'workspace' | 'home'
| 'project'`) or nothing at all (`kind: 'empty'`). Clicking it opens a
centred command dialog carrying `WorkspaceSwitcherMenu` — that dialog's own
chrome is `command.tsx`'s already-captured surface
(`crowbar_ui::components::command`, whose own fixture is documented as *"the
live workspace switcher, reached from the Context Pill"*), and its own item
content is `workspace-switcher.md`. This file's own job is the pill itself.

## 1. Values

| React / Tailwind | Compiles to | gpui / `crowbar-ui` |
|---|---|---|
| `px-2` (wrapper) | 8px | `OUTER_PADDING_X` |
| `pb-1` (wrapper) | 4px | `OUTER_PADDING_BOTTOM` |
| `pt-0` (wrapper) | 0px | literal `px(0.0)`, no constant |
| `gap-2` (trigger) | 8px | `TRIGGER_GAP` |
| `px-3` (trigger) | 12px | `TRIGGER_PADDING_X` |
| `py-1.5` (trigger) | 6px | `TRIGGER_PADDING_Y` |
| `rounded-lg` (trigger) | `theme.radius_lg` | reused, not re-derived |
| `bg-sidebar-element-idle` | `theme.sidebar_element_idle` | reused |
| `border` (trigger, real width, `border-transparent` colour) | 1px, transparent | `button::BORDER_WIDTH` reused, `Color::TRANSPARENT` — see §2 |
| `gap-0.5` (label stack) | 2px | `STACK_GAP` |
| `text-xs` (small line) | 12px font, computed line-height **16px** (`getComputedStyle`, `text-xs`'s own bundled ratio — `leading-tight` does *not* reach the computed property), rendered **box 15px** (`getBoundingClientRect`, what the oracle compares) — see §2 | `SMALL_TEXT`, `.line_height(SMALL_LINE_BOX_HEIGHT)` (a literal, not a ratio) |
| `text-[13px]` (large line) | 13px font, computed line-height **16.25px** (`leading-tight`'s own `1.25`, inherited — `text-[13px]` has no ratio of its own), rendered **box 16px** — see §2 | `LARGE_TEXT`, `relative(LEADING_TIGHT)` (gpui's own rounding lands on the right box unaided) |
| `size="lg"` on `<RepoAvatar>` | `repo_avatar::Size::Lg` | reused directly (`RepoAvatar::render`) |
| `<Library size={14}>` | 14px, unpainted | `LIBRARY_SIZE` |
| `hover:bg-sidebar-element-hover` | a colour-only rule | not modelled — no field on this contract |

## 2. Three defects, and the instrument that finally separated them

**The border.** `button-variants.ts`'s own base class list is
`"...rounded-lg border font-medium..."` — every `<Button>`, `ghost`
included (`'border-transparent text-foreground hover:bg-accent...'`),
carries a real 1px border, coloured transparent for this variant. The first
version of this port drew no border at all, so `border.w` compared `0`
against the reference's `1` — exact, not tolerance-gated (`ANCHORS.md`
§5). Fixed by reusing `button::BORDER_WIDTH` with `Color::TRANSPARENT`.
Because Tailwind's `box-sizing: border-box` puts this border *inside* the
box's own height (unauthored `h-auto` means the box is simply `border +
padding + content` regardless of `box-sizing`), adding it without touching
anything else moves the trigger's own height *up* by 2px — the wrong
direction on its own, which is why this defect had to be diagnosed
together with the next two rather than fixed in isolation.

**The large line's line-height.** `text-[13px]` carries no paired
`line-height` utility (compiled to confirm: `.text-\[13px\] { font-size:
13px; }`, nothing else), so it takes whatever it inherits. The label
stack's own wrapper carries `leading-tight` (`--leading-tight: 1.25`,
compiled directly rather than trusted as Tailwind's stock value), and a
unitless `line-height` inherits as the *number* per CSS2.1 §10.8.1, not a
fixed pixel value — recomputed against each descendant's own font size.
`workspace_switcher::CONTENT_HEIGHT`'s own 18px, borrowed here in the
first version of this port on the theory that `context-pill.tsx`'s
`font-mono` and `command.tsx`'s `font-editor` are the same font family
(they are — both `var(--editor-font-family)`), was the wrong number to
borrow: that file's own `text-[13px]` has no `leading-*` ancestor, so its
18px answers a different question than this one's computed `1.25 × 13 =
16.25px`.

**The small line: a computed property and a rendered box are two different
numbers, and this file spent two more passes finding that out.** Fixing
the border and the large line's *computed* value still left the trigger a
whole pixel over the reference — 48px against 47. Two attempts at that
pixel each measured the wrong thing:

1. **First**, the small line was moved onto `leading-tight`'s own `1.25`
   outright, on the theory that `--tw-leading` inherits into it after all
   despite `@property --tw-leading { inherits: false }`. That produced an
   exact 47px match and was reverted once a direct `getComputedStyle` read
   of the live DOM showed the small line's own **computed** `line-height`
   really is `text-xs`'s own `1.3333` (`16px` on a `12px` line), not
   `1.25`. Right instrument, and it said the fix was wrong — because it
   was answering a different question than the one that mattered.
2. **Second**, reverting to the correct computed ratio put the trigger back
   at 48px, which read as a genuine, unexplained residual — until the same
   two live text leaves were measured a second time, on `bounds.h`
   (`getBoundingClientRect`, what `ANCHORS.md` actually compares) rather
   than `getComputedStyle().lineHeight`:

   ```
   "oracle-fixture"  font-size 12px   computed line-height 16px      rendered box 15
   "home"            font-size 13px   computed line-height 16.25px   rendered box 16
   ```

   A CSS engine's *used line-box height* — the quantity that determines
   how tall an inline box actually renders — is derived from the font's
   own ascent, descent and half-leading (CSS2.1 §10.8), not simply copied
   from the `line-height` property's computed value. For JetBrains Mono
   Variable at these two sizes, that formula lays out a box one pixel
   short of the computed property on the small line (`16 → 15`), and the
   same one-pixel gap on the large line (`16.25 → 16`) disappears into
   gpui's own `.round()` in `text_system.rs`'s `line_height_in_pixels`
   without needing a second constant. The arithmetic then closes exactly:

   ```
   15 (small box) + 2 (stack gap-0.5) + 16 (large box)  = 33   inner span, confirmed live
   33 + 6 + 6 (py-1.5) + 1 + 1 (border)                 = 47   trigger, confirmed live
   ```

`context_pill.rs`'s own `SMALL_LINE_BOX_HEIGHT` (`px(15.0)`) is a literal,
not a ratio — deliberately, since no ratio this port has a name for
produces it: it is a font-metrics fact, taken as measured the way
`repo_avatar.rs` takes a caller's `avatar.color` as measured rather than
invented. `TEXT_XS_LINE_HEIGHT` (`text-xs`'s own `4/3` ratio) stays in the
file as the small line's real, confirmed *computed* value, no longer read
by `stack()`'s own render path but exercised by a test that asserts the
two numbers disagree, so the distinction stays checked rather than merely
asserted in a comment. `LEADING_TIGHT` needed no equivalent literal: on
the large line the ratio *is* the right computed value, and gpui's own
rounding happens to land on the correct rendered box without help.

**The lesson, from both sides.** The port's own author flagged from the
start that the small-line finding was "not independently confirmed against
the browser engine" — right to say so. The first correction came from
`getComputedStyle`, the CSS-property instrument, and it was right about
the property and wrong about what to build, because the oracle does not
compare properties — it compares `bounds.h`, the second, different
instrument. Two correct-looking measurements pointed the fix in opposite
directions because they were answering two different questions; only
reading `getBoundingClientRect` on the same two nodes settled which one
the port actually needs to match.

## 3. `scale-110` is not modelled

`context-pill.tsx` wraps its trailing glyph in `<span className="flex
shrink-0 scale-110">`. CSS `transform: scale()` does not participate in
layout — the scaled element keeps its own untransformed box for every
purpose a box tree cares about — but it **does** move
`getBoundingClientRect()`'s answer, which reports the painted, post-
transform box. gpui has no paint-time-only scale on `Styled` that leaves the
layout box alone the way CSS's does. `ContextPill::render` renders the icon
at its own natural extent; a future oracle comparison on this one anchor's
`w`/`h` will show a `size × 0.10` delta that is the contract's own gap, not
a port defect.

## 4. `kind: 'empty'` is not modelled

`ContextPill()` returns `null` in that cell — there is no element to
measure, so `crowbar_ui::components::context_pill::ContextPill` has three
variants, not four.

## 5. Anchoring

`context-pill.tsx` carried no `data-oracle-id` before this item. Two are
added:

* `context-pill` on the outer wrapper `<div>` — this surface's own root.
* `context-pill-trigger` on the `<Button>` inside `CommandDialogTrigger`'s
  `render` prop, overriding `button.tsx`'s own `data-oracle-id: 'button'`
  default. Namespaced for the same reason `sidebar-project-header`'s own
  four buttons are: nesting `Button::render`'s own `anchors.root(...)`
  inside this surface's root would contest which anchor `ANCHORS.md` §4
  means, so this port builds its own box from `button::Size`/`RadiusClass`'s
  public values instead — and the React id has to stop colliding with the
  generic `button` surface for the same reason.

Composed, not authored here: `repo-avatar` (`RepoAvatar::render`),
`workspace-branch-icon` (`WorkspaceBranchIcon::render`, and, on the
`working` branch, `flicker-spinner` one level deeper). No
`oracleSurfaceScope` entry needed — every nested anchor this composition can
reach is one it actually paints, never left unpainted the way
`sidebar-project-header`'s toggle icon is.

## 6. Declarations

`CONTENT_SIZED = []`. `LINE_SIZED = []` — every box here is authored
(padding/gap), never a bare text run's own line box exposed as the anchor's
height.

## 7. The state axis

Every one of the six §8.3 flags is unmodelled — the trigger's own
`hover:bg-sidebar-element-hover` is a colour-only rule with no field on this
contract, and there is no `focus:`/`data-active` rule at all.
`--kind`/`--status`/`--working`/`--avatar` are this surface's own axis
instead, the `fps-overlay` shape. `Params::no_state_axis()` returns `true`.

## 8. `row_layout` coverage

* the default cell carries the root, the trigger, and the status glyph's own
  nested anchor, never `repo-avatar`
* `--avatar` swaps the status glyph for `repo-avatar`; `--working` swaps it
  back to the spinner even with `--avatar` set (the two-step precedence)
* `--kind home`/`--kind project` never carry `repo-avatar`
* the trigger sits inset by the wrapper's own `px-2`/`pt-0`
* the root's own width tracks `--width` exactly
* the live parity cell (`--kind home --width 344`) is driven and checked
  against the reference on every run: trigger height `47px` (±0.5), root
  height `51px` (±0.5), `border.w` exactly `1`, `border.color.a` exactly
  `0` — all four pass, §2's own three-defect account held as a permanent
  regression

## 9. Reachability

`ide-shell.tsx` → the sidebar column, mounted unconditionally above
`sidebar-tab-bar.tsx`.
