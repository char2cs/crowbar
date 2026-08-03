# `context-pill` (P3.58)

`web/src/components/layout/context-pill.tsx` →
`crates/crowbar-ui/src/components/context_pill.rs`,
`crates/crowbar-app/src/surfaces/context_pill.rs`,
`crates/crowbar-app/src/row_layout/context_pill.rs`.

**A live reference exists — not captured by this item, but landed against
it — and it is not fully reconciled yet.** The item that ported this
surface drove no oracle and captured no snapshot, and every number in the
first version of this file was read off the app's own compiled Tailwind
alone. A parity run taken afterward, against `--surface context-pill --kind
home --width 344`, found the trigger 1px taller than the reference and
missing a real border. Two real, confirmed defects are fixed (§2): a
missing 1px transparent border, and a wrong line-height on the label
stack's large line. **A third, one-pixel residual is open and unexplained**
— fixing those two alone still leaves the trigger at 48px against the
reference's 47. An intermediate attempt closed that gap by also moving the
stack's *small* line onto the large line's ratio; a direct
`getComputedStyle` read of the live DOM proved that wrong (the small line
really does keep its own `text-xs` ratio) and it was reverted rather than
kept as a passing test with a disproved mechanism. §2 carries the account
in full, and `row_layout::context_pill::the_live_parity_cell_matches_the_
reference_within_tolerance` is **currently failing on purpose** — height
48px against an expected 47±0.5 — until whatever produces the missing
pixel is found for real.

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
| `text-xs` (small line) | 12px font, **16px line** — `text-xs`'s own bundled ratio, confirmed live (`getComputedStyle`: `1.3333`); the ancestor `leading-tight` does *not* reach it | `SMALL_TEXT`, `relative(TEXT_XS_LINE_HEIGHT)` |
| `text-[13px]` (large line) | 13px font, **16.25px line → rounds to 16** — confirmed live (`getComputedStyle`: `1.25`); see §2 | `LARGE_TEXT`, `relative(LEADING_TIGHT)` |
| `size="lg"` on `<RepoAvatar>` | `repo_avatar::Size::Lg` | reused directly (`RepoAvatar::render`) |
| `<Library size={14}>` | 14px, unpainted | `LIBRARY_SIZE` |
| `hover:bg-sidebar-element-hover` | a colour-only rule | not modelled — no field on this contract |

## 2. Two confirmed defects, and one residual pixel still open

**The border.** `button-variants.ts`'s own base class list is
`"...rounded-lg border font-medium..."` — every `<Button>`, `ghost`
included (`'border-transparent text-foreground hover:bg-accent...'`),
carries a real 1px border, coloured transparent for this variant. The first
version of this port drew no border at all, so `border.w` compared `0`
against the reference's `1` — exact, not tolerance-gated
(`ANCHORS.md` §5). Fixed by reusing `button::BORDER_WIDTH` with
`Color::TRANSPARENT`. Because Tailwind's `box-sizing: border-box` puts this
border *inside* the box's own height (unauthored `h-auto` means the box is
simply `border + padding + content` regardless of `box-sizing`), adding it
without touching anything else moves the trigger's own height *up* by 2px
— the wrong direction on its own, which is why this defect had to be
diagnosed together with the next one rather than fixed in isolation.

**The large line's line-height.** `text-[13px]` carries no paired
`line-height` utility (compiled to confirm: `.text-\[13px\] { font-size:
13px; }`, nothing else), so it takes whatever it inherits. The label
stack's own wrapper carries `leading-tight` (`--leading-tight: 1.25`,
compiled directly rather than trusted as Tailwind's stock value), and a
unitless `line-height` inherits as the *number* per CSS2.1 §10.8.1, not a
fixed pixel value — recomputed against each descendant's own font size.
`workspace_switcher::CONTENT_HEIGHT`'s own 18px, borrowed here in the first
version of this port on the theory that `context-pill.tsx`'s `font-mono`
and `command.tsx`'s `font-editor` are the same font family (they are —
both `var(--editor-font-family)`), was the wrong number to borrow: that
file's own `text-[13px]` has no `leading-*` ancestor, so its 18px answers a
different question than this one's `1.25 × 13 = 16.25px` (gpui rounds to
16, `text_system.rs`'s own `line_height_in_pixels`).

**Both fixes together still leave the trigger 1px over the reference, and
that residual is open.** Driving `--kind home` with the border added and
the large line corrected produces 48px against the reference's 47 — not 50
(border alone), so the two fixes above are real progress, but not the
whole story.

An intermediate attempt closed the gap anyway, by also moving the *small*
line onto `LEADING_TIGHT` (`1.25 × 12 = 15px`, against `text-xs`'s own
`calc(1 / 0.75)` → `16px`). That produced an exact 47/51 match and was
**wrong**: a direct `getComputedStyle` read of both live text leaves gives

```
"oracle-fixture"   font-size 12px   line-height 16px      ratio 1.3333
"home"             font-size 13px   line-height 16.25px   ratio 1.2500
```

— the small line really is `text-xs`'s own `1.3333`, exactly as
`@property --tw-leading { inherits: false }` says it should be (the large
line's `1.25` is confirmed correct the same way — `text-[13px]` has no
line-height of its own to keep the ancestor's ratio out). Forcing the small
line to `1.25` made it 1px *short* (`15` vs the real `16`) in a way that
happened to cancel a *second*, still-unidentified 1px the trigger is over
by elsewhere — two errors reading as zero. Reverted: both lines are back to
their own confirmed-live ratios (`context_pill.rs`'s own `stack` method),
and `row_layout::context_pill::the_live_parity_cell_matches_the_reference_
within_tolerance` is failing again, on purpose, at 48px against 47±0.5.

**What has been ruled out, for whoever picks the residual pixel up next:**
the border (confirmed 1px, transparent, both sides — not a rounding
question), the vertical padding (`py-1.5`, 6px each side, no competing
`py-*` class anywhere in `button-variants.ts` to contest it), the stack's
own `gap-0.5` (2px, uncontested), and both line-heights individually
(`16` and `16.25`, both confirmed against the live DOM, not derived alone).
None of those four is where the missing pixel is. Worth checking next,
roughly in order of how cheaply each can be ruled out: whether gpui's own
`.round()` in `line_height_in_pixels` (which turns the large line's
`16.25` into `16`) is the right direction, given the reference's own
`bounds.h` is an exact `47` and a fractional line box is being resolved to
a whole pixel *somewhere*, on one side of the port or the other; and the
outer flex row's own structure — `context-pill.tsx`'s `--kind home` branch
nests the stack and the glyph inside a *second* `flex items-center gap-2`
span this port collapses into `trigger_shell`'s own single row, which
should be geometrically inert (no border/padding/background of its own)
but has not been independently checked.

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
  against the reference on every run — `border.w` exactly `1` and
  `border.color.a` exactly `0` both pass; the height assertions (trigger
  `47px` ±0.5, root `51px` ±0.5) **currently fail**, by design, recording
  §2's own open one-pixel residual rather than a passing test built on a
  mechanism the live DOM disproved

## 9. Reachability

`ide-shell.tsx` → the sidebar column, mounted unconditionally above
`sidebar-tab-bar.tsx`.
