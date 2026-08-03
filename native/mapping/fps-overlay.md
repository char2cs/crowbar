# `fps-overlay` (P3.52)

`web/src/components/layout/fps-overlay.tsx` →
`crates/crowbar-ui/src/components/fps_overlay.rs`,
`crates/crowbar-app/src/surfaces/fps_overlay.rs`.

> One of the "standalone sidebar chrome" cluster's five independent files
> (`native/mapping/layout-denominator.md` §8, Cluster 3). Kept in its own file
> for the reason `skeleton.md` and `toast.md` both give.

**Reference: none, and for a stronger reason than either of this port's other
two "no reference" members.** See §1.

## 0. Two premises to correct before anything else

**Not dev-only.** `native/mapping/layout-denominator.md` §4 already checked
this: `showFpsOverlay` is a real `features/settings/types/settings.ts` field,
defaults `false`, and is exposed as a toggle in
`features/settings/components/tabs/developer-settings.tsx` — a Developer
settings tab, not a build flag. There is no `NODE_ENV`/`import.meta.env` check
anywhere in the file or its gate. It ships in every build; a user who opens
Settings → Developer and flips the switch sees it.

**Not already covered.** No existing surface anchors this component or
anything shaped like it; `grep -rn fps-overlay native/crates native/mapping`
before this item found nothing outside the survey itself.

## 1. Why there is no reference: the file carries no `data-oracle-id` at all

`skeleton.tsx` and `toast.tsx` each write `data-oracle-id` on their own
primitive and are unreachable for a *narrower* reason (a `<Suspense>`
fallback that never mounts; a manager nobody calls `.add()` on).
`fps-overlay.tsx` is unreachable for a simpler, stronger reason:

```
$ grep -n data-oracle-id web/src/components/layout/fps-overlay.tsx
$
```

Nothing. The `<div>` and every one of its seven `<span>` children carry no
oracle attribute in any state, on any build. There is no anchor id the React
extractor could ever collect here — not "the fallback never mounts" (an
absence with a cause a future frontend change could remove), but "the
attribute was never written" (an absence that stays true until someone adds
one). No reference was captured, attempted through a synthetic trigger, or
fabricated; there is no `/tmp/p3-ref-fps-overlay.json`.

## 2. Values

| React / Tailwind | Compiles to | gpui / `crowbar-ui` | Oracle |
|---|---|---|---|
| `bottom-8` | `calc(var(--spacing) * 8)` = **32px**, off the window's own bottom edge (`position: fixed`) | `BOTTOM` | no field exists to compare it against |
| `right-3` | **12px**, off the window's own right edge | `RIGHT` | — |
| `px-2.5` | **10px** | `PADDING_X` | — |
| `py-1.5` | **6px** | `PADDING_Y` | — |
| `rounded-md` | `theme.radius_md` = **8px** | `theme.radius_md.value()` | — |
| `mx-1.5` (each `·`) | **6px** each side | `SEPARATOR_MARGIN_X` | — |
| `text-[11px]` | an arbitrary value, not a `ui-text-*` step | `FONT_SIZE = px(11.0)` | — |
| `leading-none` | `line-height: 1` | not separately modelled — gpui derives the line box from the font | — |
| `font-mono` | `var(--font-mono)` = `'JetBrains Mono Variable', ui-monospace, monospace` | `theme.font_mono.primary().unwrap_or("monospace")` — the same one honest limit `file_tree_row.rs`/`inline_error.rs` already record | — |
| `tabular-nums` | `font-variant-numeric: tabular-nums` | unmodelled — no comparable field, and no font-feature call site elsewhere in this tree needed one | — |
| `shadow-xl` | a box-shadow preset | `.shadow_xl()` — painted for fidelity; `ANCHORS.md` §6 has no shadow field either way | — |
| `style={{ background: 'rgba(0,0,0,0.72)' }}` | **not a Tailwind utility at all** — the one raw-colour inline style in this whole five-file cluster | `Color::BLACK.mix(72.0, Color::TRANSPARENT)` — see §3 | — |
| `style={{ backdropFilter: 'blur(10px)' }}` | no gpui equivalent (§6.3) | absent | — |
| `fps >= 110`/`>= 80`/`> 0`/else | `success`/`warning`/`destructive`/`muted-foreground` | `FpsTier::of` → `theme.{success,warning,destructive,muted_foreground}` | — |
| `drops > 0` | `text-destructive font-medium` vs `text-muted-foreground` | one boolean picking colour and weight together | — |

## 3. `Color::BLACK`, minted for this component

`rgba(0,0,0,0.72)` is exactly `color-mix(in srgb, black 72%, transparent)` —
the same idiom `text-muted-foreground/72` compiles to elsewhere in this tree
(`tabs.md` §1). `theme.css` has no `--black` custom property to seal this
from, so `Color::BLACK` is minted in `theme/token.rs` the same way
`Color::WHITE` already is for `slider.tsx`'s `bg-white`: a literal naming a
CSS keyword, not a value read out of the stylesheet. `check-invariants.sh`
rule 4 permits it because the literal lives inside
`crates/crowbar-ui/src/theme/`, the one directory the rule exempts.

## 4. The one structural deviation, and the harness defect it caught

`fps-overlay.tsx`'s `<div>` carries no `flex` class — its seven `<span>`s flow
inline by CSS default. gpui has no inline layout model, so the shell is built
`.flex().flex_row().items_center()` instead — the same substitution
`tabs.rs`'s module docs record for `TabsList`'s own CSS defaults and
`toast.rs` §5 records for `Toast.Positioner`.

**A real bug this item's own row_layout tests caught and fixed, recorded
because it very nearly shipped:** the first draft of `FpsOverlay::shell`
built the padded, coloured box but never actually called `.absolute()`. It
compiled, and it rendered a badge — just one that stretched to the full
window width and sat flush at the harness's own top inset, because a plain
`div()` inside a flex-column parent with no explicit width stretches to fill
the cross axis by taffy's default `align-items: stretch`. The first
`row_layout::fps_overlay` run caught it immediately: `size.width` equal to
the viewport, not content-sized. Fixed by adding `.absolute().bottom(BOTTOM)
.right(RIGHT)`, which is also when the second defect below surfaced.

**taffy's containing block for an absolutely-positioned element is the
immediate parent, unconditionally — not "the nearest ancestor CSS would call
positioned."** With `.absolute()` added, `render_row`'s own wrapper
(`div().w(cell.width_px())`, no explicit height, no `.relative()`) became the
badge's containing block by virtue of being its immediate parent. An absolute
child contributes nothing to a parent's own auto-height, so with no other
content the wrapper collapsed toward zero height and `bottom: 32px` landed
the badge 32px *above* a near-zero box — off the top of the window,
`visible: false`. `crates/crowbar-app/src/surfaces/fps_overlay.rs::render`
now hands the component a stage explicitly sized to
`INSET_Y + cell.window_extent()` (the room the harness's own top inset leaves
below it), which lands the stage's bottom edge exactly on the window's true
bottom. This is a fact about the **row_layout harness's own wrapping**, not
about the shipping app — `IDEShell`'s real root is already window-sized, so
the question this section answers never comes up there — and it is recorded
in `crowbar-app/src/surfaces/fps_overlay.rs`'s module docs rather than
`crowbar-ui`'s, which stays agnostic of it.

## 5. What "content_sized" means here, and the one thing it is not

`ID_FPS_OVERLAY` is declared `CONTENT_SIZED`: no `w-*`/`h-*` class anywhere on
the root, so its box is `px-2.5 py-1.5` around whatever the seven runs
measure — the same "padding plus content, no authored length" shape
`keybinding::CONTENT_SIZED` already declares. It is **not** declared
`LINE_SIZED`: the box's height is padding plus a *multi-run* line (seven
independently-styled children on one row), not one text node's own line box,
and `ANCHORS.md` v1.6 is written for a single anchored run — declaring it
here would be the `tabs` tab trap in a new shape.

## 6. The seven runs are plain, unanchored children

None of the seven `<span>`s carries a `data-oracle-id` in the source, so each
is rendered as a plain `div()` child rather than routed through
`AnchorSink::text_half` — the same choice `tabs.rs` makes for a tab's
call-site label. Routing any of them through the sink would record a text
group under the box's own id that the reference has no field for at all.
`record.text` on the one real anchor (`fps-overlay`) is therefore always
`None`, the same shape `skeleton.rs`'s own precedent establishes for a
placeholder that paints no text of its own.

## 7. What the state axis is here, and what it is not

Every one of the §8.3 flags is unmodelled: `pointer-events-none`,
`aria-hidden`, nothing interactive, no `hover`/`focus`/`selected`/`empty`
original. `--fps`/`--max-dt`/`--drops` are this surface's own axis instead —
the same shape `tabs`' `--tabs`/`--active` are. `layout-denominator.md` §4's
own framing applies exactly: the *anchors* this surface has (badge position,
the four-way colour ladder, the drop counter's colour-and-weight pairing) are
exactly as drivable as any other state-driven primitive; only the raw
frame-timing number itself is real-time instrumentation the oracle cannot
reproduce identically, and nothing here tries to.

Checked exhaustively rather than assumed, the same standard
`workspace-branch-icon.md` sets: `export function FpsOverlay()` takes **no
props at all** — no `className`, no prop spread, nothing a call site could
merge down to an edge value the way `avatar.rs`'s `--tone error` or
`flicker_spinner.rs`'s unreached `empty` still can. Every `className` in
`fps-overlay.tsx` is a fixed string. `Empty` does not apply on its own
narrower terms either: its §8.3 meaning is a *row's* trailing edge carrying
no badge or count, modelled on `git-status-row` alone, and this surface is
not a row. So all four non-mandatory flags are unmodelled with no seam left
to reach through any hypothetical caller — a stronger case than
`workspace-branch-icon`'s, which still forwards three real props
(`status`/`working`/`isPlaceholder`) it merely never routes to a
`className`. `fps-overlay`'s `Params` therefore declares
`SurfaceParams::no_state_axis() -> true`, the same declaration
`workspace_branch_icon.rs` introduced (P3.50) and the shared invariant in
`surface.rs` now requires of every surface with zero real flags.

## 8. Declarations

`CONTENT_SIZED = [fps-overlay]`. `LINE_SIZED = []`. See §5.

## 9. Mutation evidence

`the_badge_sits_a_fixed_distance_off_the_windows_own_corner` asserts the
right/bottom gaps against the **literal** `px(12.0)`/`px(32.0)`, not against
`RIGHT`/`BOTTOM` — a first draft that compared against the constants
themselves passed even after `BOTTOM` was mutated to `SPACING * 5.0` (20px),
because the component paints with the same constant the test would have been
comparing it to. Confirmed live: mutating `BOTTOM` to `SPACING * 5.0` with
the literal assertion in place fails as `expected 32px, got 20px`; reverting
restores green. The same class of guard is why `the_box_carries_its_own_padding_and_radius`
asserts `record.radius` against the literal `px(8.0)` rather than
`theme.radius_md.value()`.
