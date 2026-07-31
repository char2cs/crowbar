# `callout-node` (P3.10)

`web/src/components/ui/callout-node.tsx` →
`crates/crowbar-ui/src/components/callout_node.rs`.

Reference: `/tmp/p3-ref-callout-node.json`, captured live at
`callout-node · 1714 · dark · normal`, four anchors, `720 × 68.58`.

## 0. Reachability — 0 live, then 1, and the fixture is data

`[class*=callout]` measured **0** in the running app at rest. A callout renders
from a markdown file whose blockquote opens with a GitHub alert marker, so one
was written into the fixture workspace:

```
<fixture workspace>/callout-fixture.md

  # Callout fixture

  > [!NOTE]
  > Reachability
```

Opening it in the app renders `CalloutElement` through
`markdown-callout-rules.ts`'s `blockquote` deserializer. **Live count after: 1.**

That is a *user path*, not a harness — the file is ordinary data, the same
standing as P3.3's agent reply and Phase 1's dirty git fixture — and **deleting
the file removes the only capturable Callout.**

> **Editor focus is NOT required, and the brief expected it to be.**
> `document.hasFocus()` is `false` and immovable, and that is what blocks
> `separator` (Plate's `FloatingToolbar`, gated on `useEventEditorValue('focus')`).
> It does **not** gate node rendering: a Plate element renders from the document,
> and the document is on disk. Measured — the callout, its emoji control and its
> paragraph all laid out with focus false throughout.

## 1. ⚠ F3 in its purest form: a **stylesheet** beats the entire class list

`MAPPING.md`'s standing rule is "compile the CSS, do not read the class name".
**On this component that rule is not enough either**, because a descendant
selector outranks every utility. The JSX writes

```
my-1 flex rounded-sm bg-muted p-4 pl-3
```

and `features/editor/markdown/plate/markdown-editor.css:223` answers

```css
.crowbar-markdown-editor .slate-callout {
  margin-block: 1.2em;
  padding: 0.85rem 1rem;
  border-radius: var(--radius-md);
  background: var(--md-wash);
}
```

Measured on the live element, **every one loses**:

| React / Tailwind | reads as | **renders as** | gpui / `crowbar-ui` | Oracle |
|---|---|---|---|---|
| `my-1` | 4px | **19.19px** (`1.2em`) | `MARGIN_BLOCK_EM` (not drawn — see §4) | invisible |
| `rounded-sm` | 6px | **8px** (`--radius-md`) | `theme.radius_md` | compared |
| `bg-muted` | `--muted`, white @ 4% | **`--md-wash`**, foreground @ 5% | `color_foreground.mix(5.0, TRANSPARENT)` | compared |
| `p-4` | 16px | **13.6px** block / 16px inline | `PADDING_Y` / `PADDING_X` | compared |
| `pl-3` | 12px | **16px** — overridden with the rest | `PADDING_X` | compared |
| `flex` | flex | flex | `.flex()` | invisible |

Only `flex` survives. **Both earlier defences fail here** — reading the class
name fails, and compiling the utility fails too. The value has to come off the
running element, and this is the first component in the port where that is the
*only* method that works.

## 2. `--md-wash` is derived, not minted

`--md-wash` is `color-mix(in oklch, var(--foreground) 5%, transparent)` and lives
in `markdown-editor.css`, which F5 lists among the `--md-*` properties that are
**not** ported into the sealed table. So it is computed the way
`file_tree_hover_bg` is, through `Color::mix`.

The mix spaces differ — `Color::mix` is `in srgb`, the stylesheet says
`in oklch` — and here that is provably immaterial: the second colour is
`transparent`, so the operation only scales alpha and both spaces return the
source colour at 5%. Pinned against the reference's `#f5f5f50d` by
`the_wash_is_the_foreground_at_five_percent` rather than argued.

## 3. Two findings on the emoji control, and the first contradicts the record

| | |
|---|---|
| **`radius 10`, unmerged, `visible: true`** | P3.1 recorded that "**no live Button is both unmerged and visible**", with the only two candidates sitting in the carousel's snapped-out panels. **This one is neither.** The callout's control overrides the box (`size-6 p-1 text-[18px]`) but writes no `rounded-*`, so it keeps the primitive's `rounded-lg`, and it is on screen. The reference says `radius 10, visible true`. |
| **`24 × 32`, not the square its class reads as** | `sm:h-8` takes the height off `size-6` and leaves the width alone — `size-*` and `h-*` are different tailwind-merge groups and Tailwind emits the `sm:` variant later. The `sm:` trap in **one axis only**. |

`text-[18px]` is dead the same way: the variant's `sm:text-sm` is emitted later,
so the reference reads `size 14` on a `line_height` of `20`.

## 4. What is ported and what is not

| Thing | Status |
|---|---|
| `margin-block: 1.2em` | **recorded, not drawn.** `bounds` is a border box and the surface's own root is what a margin would move — it would offset every anchor by a constant and prove nothing. `MARGIN_BLOCK_EM` carries the value and the layout tests assert it. |
| the `.slate-p` body | **drawn, unanchored.** It is `ParagraphElement`'s node, not this component's (v1.8). Drawn because the callout's height is its padding plus its content. |
| the emoji popover | **absent.** `EmojiPopover`/`EmojiPicker` are a base-ui overlay; no anchor, no reference (the overlay needs a click and `rAF`-throttled opacity). |
| Plate selection (`slate-selectable`) | **absent.** Editor state with no native equivalent, and undriveable on the reference side for the same `document.hasFocus()` reason. |

## 5. ⚠ A product defect: the trailing-gap rule cannot reach this component

`markdown-editor.css:327` ends with

```css
.crowbar-markdown-editor
  :is(.slate-blockquote, .slate-callout, .slate-th, .slate-td, li)
  > .slate-p:last-child { margin-bottom: 0 }
```

— a **direct-child** combinator. `CalloutElement` wraps `{children}` in two
divs, so the paragraph is a *grandchild* and the rule does not match. Measured:
the last paragraph in a callout keeps `--md-gap` (`0.9em` = **14.39px**) where
the same paragraph in a blockquote gets **0**.

The port reproduces what renders, not what the stylesheet intended.
`BODY_GAP_EM` records which is which. **This is a real, if cosmetic, defect in
the reference app** and is not a porting decision.

## 6. Declarations

`CONTENT_SIZED` and `LINE_SIZED` are both **empty**, and both are measurements:

- nothing is content-sized — the callout takes the editor's measured column and
  both inner boxes are `w-full`;
- nothing is line-sized — the emoji control is the only anchor carrying a font,
  and its box is **32px against a 20px line box**. Declaring it would
  manufacture a 12px delta on the one row with something to compare.
  `the_emoji_box_is_authored_not_line_sized` asserts the distance.

## 7. ⚠ A measured engine difference: gpui snaps `0.85rem` padding to 13.5

`padding: 0.85rem` is **13.6px**. WebKit keeps it (the reference reads the row
inset at `13.59`); gpui snaps padding to the device grid and lands on **13.5** at
DPR 2. **Δ 0.1**, comfortably inside §5's ±0.5, and it propagates to the
callout's height (`68.5` against the reference's `68.58`).

Recorded rather than corrected, and **no rule is proposed**: this is the same
shape as v1.10's intrinsic-ratio note — a quantisation both engines are entitled
to, inside tolerance today. The bound scales as `1/(2·dpr)`, so at DPR 1 the
padding could land 0.4 out and the height 0.8 — **which would exceed tolerance on
`bounds.h`**. If a DPR-1 run ever fails this surface, this is the cause.

## 8. ⚠ `Button::render` cannot be nested, and this is the first surface that could hit it

`Button::render` opts the button in through `AnchorSink::root`, which on the
driver-backed sink is `crowbar_driver::anchor_root` — and that **clears the
registry** as it enters `prepaint` (that is what makes a snapshot one frame).

A `Button::render` nested inside another surface therefore **discards every
anchor laid out before it**, silently: the snapshot is well-formed and its root
is simply gone, which reads as a port that forgot to anchor half its tree.

P3.1 could not meet this — all nine live Buttons are the whole surface. Both
P3.10 container surfaces contain one, and both avoid it by composing from
`button`'s **public values** rather than from a `Button`. No API was added to the
primitive, because it would have had exactly one caller; the hazard is recorded
here and in both components' docs instead.

**If a future container surface wants a real `Button` child, `button.rs` needs a
`render_nested` that uses `boxed` instead of `root`.** That is the orchestrator's
call, not a worker's.
