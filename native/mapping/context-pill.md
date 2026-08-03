# `context-pill` (P3.58)

`web/src/components/layout/context-pill.tsx` →
`crates/crowbar-ui/src/components/context_pill.rs`,
`crates/crowbar-app/src/surfaces/context_pill.rs`,
`crates/crowbar-app/src/row_layout/context_pill.rs`.

**A live reference exists — not captured by this item, but landed against
it.** The item that ported this surface drove no oracle and captured no
snapshot, and every number in the first version of this file was read off
the app's own compiled Tailwind alone. A parity run taken afterward, against
`--surface context-pill --kind home --width 344`, found two real defects: a
missing 1px transparent border, and a wrong line-height on the label
stack's large line (see §2). Both are fixed and covered by
`row_layout::context_pill::the_live_parity_cell_matches_the_reference_
within_tolerance`, which reproduces the reference's own numbers — root
51px, trigger 47px, `border.w` 1 — to the pixel. This file is corrected in
place rather than left describing the pre-fix state.

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
| `text-xs` (small line) | 12px font, **15px line** — not `text-xs`'s own bundled ratio; see §2 | `SMALL_TEXT`, `relative(LEADING_TIGHT)` |
| `text-[13px]` (large line) | 13px font, **16.25px line → rounds to 16** — see §2 | `LARGE_TEXT`, `relative(LEADING_TIGHT)` |
| `size="lg"` on `<RepoAvatar>` | `repo_avatar::Size::Lg` | reused directly (`RepoAvatar::render`) |
| `<Library size={14}>` | 14px, unpainted | `LIBRARY_SIZE` |
| `hover:bg-sidebar-element-hover` | a colour-only rule | not modelled — no field on this contract |

## 2. Two defects a live parity run found, and the arithmetic that resolved them

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

**Both fixes together still left the trigger 1px over the reference.**
Driving `--kind home` with the border added and the large line corrected
produced 48px against the reference's 47 — not 50 (border alone) and not
47 (the fix in full), a real, distinct third finding: the *small* line
also needed `LEADING_TIGHT`, not `text-xs`'s own bundled `calc(1 / 0.75)`
ratio. Per the CSS spec, that should not happen — `text-xs`'s own
`line-height: var(--tw-leading, var(--text-xs--line-height))` is declared
next to `@property --tw-leading { inherits: false }`, which should keep
the ancestor's `leading-tight` from reaching it at all and leave `text-xs`
reading its own fallback ratio. It does not match what the reference
shows. The most likely account: the WKWebView build the reference was
captured from does not implement `@property`'s `inherits: false` for this
property, so `--tw-leading` inherits as an ordinary (unregistered) custom
property would, and reaches the small span after all. **Not independently
confirmed against the browser engine** — the reference's own numbers are
the evidence for this account, not a second, checked source, and it is
recorded as a finding rather than papered over. Both stack lines now
render at `relative(LEADING_TIGHT)`, and the live cell matches to the
pixel: root 51px, trigger 47px, `border.w` 1
(`row_layout::context_pill::the_live_parity_cell_matches_the_reference_
within_tolerance`).

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
* the live parity cell (`--kind home --width 344`) matches the reference to
  the pixel: trigger `47px` (±0.5), root `51px` (±0.5), `border.w` exactly
  `1`, `border.color.a` exactly `0` — §2's own finding, held as a permanent
  regression

## 9. Reachability

`ide-shell.tsx` → the sidebar column, mounted unconditionally above
`sidebar-tab-bar.tsx`.
