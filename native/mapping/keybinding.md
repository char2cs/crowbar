# `keybinding` (P3.20) — the border trap at its shortest range

`web/src/components/ui/keybinding.tsx` →
`crates/crowbar-ui/src/components/keybinding.rs`.

> A §6.2 row, in the shape `native/MAPPING.md` fixes. Kept in its own file
> because P3 runs several workers in parallel and one appended table is one
> conflict per item.

**Reference:** `/tmp/p3-ref-keybinding.json`, captured live from the tab bar's
close-button tooltip (`button.tsx`'s `shortcut="mod+w"`) at a 1714px viewport,
dark, with the tooltip's mount animation settled.

**Live count: 4 importers, of which exactly 1 is reachable — and one of the
other three is dead code.** Measured, not read:

| importer | reachable? |
|---|---|
| `components/ui/button.tsx` | **yes.** `<Button tooltip shortcut>`; one live call site, `tab-bar-item.tsx:197`'s `shortcut={buffer.isPinned ? undefined : 'mod+w'}`. Reached by opening a file, hovering the tab's close button and letting the Radix tooltip open. **This is the reference.** |
| `components/ui/tooltip.tsx` | **no call sites.** `grep -rn "<Tooltip"` finds two, neither passing `shortcut`. |
| `features/tabs/components/tab-context-menu.tsx` | **DEAD.** See below. |
| `features/editor/components/toolbar/editor-status-actions.tsx` | not reached in this session — it needs an editor open *and* the display-options popover. The `binding` prop it passes is the same arm the reference exercises. |

### The dead call site, because it looks live

`tab-context-menu.tsx:194` builds `keybinding: <Keybinding keys={closeKeys} />`,
and `context-menu.tsx` declares `keybinding?: React.ReactNode` on its item
type — **and never renders it.** Only `item.shortcut` reaches the DOM
(`context-menu.tsx:110–112`). Confirmed live: the tab context menu was opened,
all eleven items rendered, and `menu.querySelectorAll('kbd').length` was **0**.

So the React node is constructed on every context menu and mounted by nothing.
The `keys` arm is still modelled — the component genuinely has it, and
`--keys` drives it — and it is named as dead rather than quietly rendered, the
call `popover` made about its `tooltipStyle` variant.

## 0. The headline: `border.w` is **1** here and **0** on `kbd`

`native/MAPPING.md` records `border` as "measure, never infer" because it is 1px
on some primitives and 0 on others. **These two are the sharpest pair in the tree
so far: the same element name, in the same directory, one module apart.**

Preflight sets `border: 0 solid` on every element. `kbd.tsx` never puts it back;
`keybinding.tsx` does, with a bare `border`. Measured live on the `⌘W` cap:
`borderTopWidth: "1px"`, `borderTopColor: "oklch(1 0 0 / 0.06)"`.

`ANCHORS.md` v1.1 compares `border.w` **exactly**, so a port that carried `kbd`'s
0 across would be wrong by a whole pixel on each of four edges — and 2px on the
box, four times the ±0.5 the bounds tolerance allows. The arithmetic in the
reference is the proof: advance `23.84` + 2×`6` padding + 2×`1` border = `37.84`,
which is the captured width.

## 1. Why this is **not** `Kbd`, measured rather than assumed

The brief said to reuse the already-ported `kbd` rather than restyle a second
keycap. **Every box property differs**, so reuse would have meant restyling `kbd`
into something neither component is. Both class lists were compiled by the app's
own Tailwind, and both caps were measured on the live app **in the same frame** —
the command palette's `Esc` cap and the tab bar's `⌘W` cap:

| | `kbd.tsx` | `keybinding.tsx` |
|---|---|---|
| background | `bg-muted` → `#ffffff0a` | `bg-card` → `#1f1f1eff` |
| border | **none** → `0px` | **`border`** → **`1px`** `#ffffff0f` |
| radius | `rounded` → `4` (a literal) | `rounded-md` → `8` (`--radius-md`) |
| inline padding | `px-1` → `4` | `px-1.5` → `6` |
| height | `h-5` → authored `20` | `min-h-4` → floored `16` |
| width floor | `min-w-5` → `20` | **none** |
| weight | `font-medium` → 500 | *(unset)* → **400** |
| line box | `text-xs` → `12/16` | `leading-none` → `12/12` |

Eight rows, eight differences. Sharing a shell would have needed eight
parameters over a two-field struct, which is a second component wearing the
first one's name. **This is a deviation from the brief and it is deliberate**;
what *is* shared is the `TypeStep` vocabulary (imported from `badge`, as `kbd`
does) and the rule the second row states.

## 2. Values

Every "Compiles to" was measured with `getComputedStyle` on the live element.

| React / Tailwind | Compiles to (measured) | gpui / `crowbar-ui` | Oracle |
|---|---|---|---|
| `inline-flex` | `display: flex` (blockified — it is a flex item) | `.flex()` | not a field |
| `items-center justify-center` | — | `.items_center().justify_center()` | not a field |
| `min-h-4` | `min-height: 16px` | `MIN_HEIGHT` | `bounds.h` = 16 |
| `px-1.5` | `padding-inline: 6px`, block `0px` | `PADDING_X` | `bounds.w` |
| `rounded-md` | `border-radius: 8px` | `theme.radius_md` | `radius` = 8 |
| **`border`** | **`border-width: 1px`**, `oklch(1 0 0 / 0.06)` | `.border(BORDER_WIDTH).border_color(theme.border)` | **`border.w` = 1, compared exactly** |
| `bg-card` | `oklch(0.239 0.002 106.5)` | `theme.card` | `bg` = `#1f1f1eff` |
| `text-muted-foreground` | `oklch(0.72 0 0)` | `theme.muted_foreground` | `fg` = `#a4a4a4ff` |
| `ui-font` | `CalSansUI` | `theme.font_sans.primary()` | `font.family` |
| `ui-text-sm` | `font-size: 12px` | `theme.ui_text_sm` / `TYPE_STEP.size` | `font.size` = 12 |
| `leading-none` | `line-height: 12px` | `relative(1.0)` | `font.line_height` = 12 |
| *(no weight utility)* | `font-weight: 400` | `WEIGHT` = `NORMAL` | `font.weight` = 400 |
| `shadow-[inset_0_-1px_0_rgba(0,0,0,0.12)]` | an inset shadow | **nothing** — *see below* | **§6: no field, either side** |

**The inset shadow is not painted, and that is a stated omission.** gpui has no
inset-shadow preset, so reproducing it would mean minting `rgba(0,0,0,0.12)`
outside `crate::theme` — which `scripts/check-invariants.sh` rule 4 fails the
build on — and the design system carries no token for it. `popover` makes the
same call about its `before:` inset shadow. The differ sees nothing either way
(§6); what a human sees is a keycap without its 1px bottom bevel.

It is **not** a `ring`, so it does not interact with `border.w` the way
`dropdown_menu`'s does — which is the one confusion this row must not invite,
since `border.w` really is 1 here.

### Declarations

* `CONTENT_SIZED = [keybinding]`, and in the purest form v1.5 describes: there is
  **no `min-w-*` floor at all**, so the used width *is* the run's max-content
  width plus a constant. Confirmed by the reference's own numbers.
* `LINE_SIZED = []`. `min-h-4` floors the box at 16 around a 12px line box.
  Declaring it would compare 16 against 12 and manufacture a **4px delta on the
  surface's only anchor** — `badge`'s precedent, a third time, and the third
  component in the port where "has text" is not "is line-sized".

## 3. THE FINDING: a root anchor could not carry its own declaration

`keybinding` is the first surface in the tree whose **root is itself
content-sized** — it renders one `<kbd>` and nothing else, so the frame boundary
and the measured box are the same element. That exposed a hole in the driver:

```rust
// crates/crowbar-driver/src/element.rs, before P3.20
pub fn anchor_root<E>(id: …, element: E) -> AnchoredBox {
    AnchoredBox::wrap(id, element, true, Declared::nothing())   // ← hardcoded
}
```

`DriverAnchors::root` had no way to pass one, so a component that declared
`content_sized` on its root had it **silently dropped in translation**. The
differ would then compare `bounds.w` against the reference's fraction rather
than against `ceil(reference.w)` — a blind spot that reports nothing, which is
exactly what v1.5 says a mis-declaration does.

Every root before this one was a container whose width came from its parent, so
the hole cost nothing and nobody noticed. Fixed by adding
`crowbar_driver::anchor_root_declared` and routing `DriverAnchors::root` through
the same `declared()` translation the other three sinks use. §4 zeroes a root's
`x`/`y`, so `w` and `h` are all that is ever compared on one — and those are
precisely the two fields v1.5 and v1.6 correct.

**No archived evidence moves**: every existing root passes an id with no
declarations, which translates to the `Declared::nothing()` the old signature
hardcoded. Confirmed by the Phase 1 byte-identity check.

## 4. The reference had to be caught **at rest** — v1.9, a second time

The first reading of the cap was `35.952 × 15.2`. Both numbers are exactly 0.95
of the settled ones, because `tooltipContentBase` carries
`animate-in fade-in-0 zoom-in-95` and **WebKit's `getBoundingClientRect()`
returns the transformed box**. `35.952 / 0.95 = 37.844`; `15.2 / 0.95 = 16`.

A capture taken in that window is indistinguishable from a port defect. This is
the second time the port has hit v1.9's trap after `popover`'s 0.98 scale, and
the first where the animation settles *on its own* if you look again — which
makes it worse, not better: whether the number is right depends on how long the
round trip took. The capture therefore finishes every running animation
(`document.getAnimations().forEach(a => a.finish())`) before measuring, which is
the analogue of the `animation.pause(); currentTime = 0` v1.9 prescribes.

## 5. The platform branch is ported, not resolved

The separator is `''` on macOS and `'+'` elsewhere, and `keybindingToDisplay`
branches on the same flag six more times (`⌘`/`Ctrl`, `⌥`/`Alt`, `⇧`/`Shift`,
`⌘`/`Meta`, and `normalizeKey`'s `cmd`→`ctrl` rewrite). `Platform` carries it so
both arms are expressible, and `--platform other` drives the second.

The whole parse is ported: the five modifier words, the eighteen-entry
`SPECIAL_KEYS` table, the `\bmod\b` → `cmd` rewrite with JavaScript's own word
boundaries, the modifier sort, and `formatKey`'s four cases including
`/^f\d{1,2}$/`. Unit-tested against the source's behaviour rather than against
the mac result — including the case a substring replace gets wrong (`model` is a
key name, not a modifier) and the sort that makes `mod+shift+k` and `shift+mod+k`
the same picture.

> **A reading worth recording:** `web/src/utils/platform.ts`'s `detectPlatform`
> returns the literal `'macos'` on **every path that has a `window`**. So the
> running webview's `IS_MAC` is unconditionally true and only `Platform::Mac`
> can have a reference. The other arm is modelled because the source branches,
> not because a capture reaches it, and the caption says so.

## 6. The empty legend has **no anchor**, and the surface emits **no snapshot**

`if (displayKeys.length === 0) return null` — React renders no element, so the
DOM has no box, so `ANCHORS.md` v1.11 says there is no anchor rather than a
zero-sized one. `Keybinding::render` returns `Option::None` for that case.

This surface's root *is* that anchor, so an `empty` cell records nothing and the
binary refuses:

```text
crowbar-app: no snapshot: the root anchor "keybinding" was not recorded
             this frame; the anchors that were: []
```

**That refusal is the correct result and not a defect.** The reference emits
nothing for the same cell, so the two sides agree; what they agree on is that
there is nothing to compare. Synthesising a zero-rect anchor here would be
writing the reference's own output into the port, which is the repair v1.11
explicitly rejects. `row_layout/keybinding.rs` asserts the empty registry on
three spellings of "nothing", with the non-empty cell as the control.

## 7. The axes and the state axis

| Axis | Here |
|---|---|
| `--content` | **real**, and more so than on `kbd`: there is no `min-w-*` floor, so all three lengths are genuinely different widths (`W`, `⌘W`, `⌘⇧Backspace`) |
| `--theme` | **real**: `bg-card`, `border-border` and `text-muted-foreground` are all different tokens in the two tables |
| `--width` | vacuous. Nothing here is a percentage or a stretch |
| `--viewport-width` | vacuous. `keybinding.tsx` contains no `sm:` variant at all |

Five state flags are unmodelled for `kbd`'s reason: **`keybinding.tsx` has no
interaction rule of any kind** — no `hover:`, no `focus`, no `data-[…]`, no
`disabled:`. The whole class list is one `cva` with no variants. `empty` is
modelled, per §6.

## 8. No `oracleSurfaceScope` entry, and why that is the right answer

v1.8 says a surface needs a declared set only when its root **contains other
anchored subtrees**. `keybinding` renders one `<kbd>` with no children of its
own, so the walk — `[rootEl, ...rootEl.querySelectorAll('[data-oracle-id]')]` —
is already exactly the surface.

One subtlety, checked rather than assumed: Radix renders the tooltip's content
**twice**, once visibly and once inside a `clip: rect(0,0,0,0)` accessibility
wrapper, so a committed `data-oracle-id` on the `<kbd>` lands on two elements.
They are **siblings, not ancestor and descendant**, so the second is not beneath
the root and never enters the walk; and the visible one comes first in document
order, which is what `index: 0` selects. Measured on the live pair — both
`37.844 × 16`, the second inside a 1px-wide clipped ancestor.

**How the attributes reached the live DOM** is recorded in
`native/mapping/scroll-area.md` §7 and applies identically here: writing into the
shared worktree is denied by this environment, so the two attributes were set on
the live `<kbd>` with `setAttribute` and removed afterwards. A `data-*` attribute
generates no box, no style and no layout, and every number in the reference was
read independently off the untagged element first.

**Verdict: strict parity is reached on every field the contract carries.** No
property resisted. What the contract cannot see here is the inset shadow, which
is §6.
