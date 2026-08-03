# `nav-stack` (P3.59)

`web/src/components/layout/nav-stack.tsx` →
`crates/crowbar-ui/src/components/nav_stack.rs`,
`crates/crowbar-app/src/surfaces/nav_stack.rs`,
`crates/crowbar-app/src/row_layout/nav_stack.rs`.

**No live reference.** This item does not run the oracle or capture a
snapshot — see the item brief's hard constraints. Every number below is read
off the app's own compiled Tailwind (`native/MAPPING.md`'s method) or reused
directly from an already-captured sibling (`sidebar_project_header`), not off
a live capture.

## 0. What this file is, and what it is not

`nav-stack.tsx` is `NavStack({ children })`: the sidebar's push/pop screen
stack. In the real app it wraps `sidebar-carousel.tsx`'s own scroll track as
`children` — `sidebar_carousel.rs`'s own module docs already record that the
carousel port "does not render [`NavStack`] at all"; this item is what fills
that gap in. `children` (and, once a screen is pushed, `screen.component`) is
opaque call-site content this port does not own, exactly the call
`sidebar-carousel`'s own four panels already made about `WorkspaceTree` /
`AgentChatsPanel` / `FileExplorerTree` / `GitPanel`.

## 1. Re-deriving `layout-denominator.md` §4's own reasoning — checked, not inherited

§4 groups this file with `sidebar-peek.tsx` as the tier's two judgment calls,
on the strength of `sidebar-carousel`'s own precedent: a store-driven wrapper
whose CSS-transition **end states** are the port target, with the transition
itself out of the oracle's reach (`ANCHORS.md` §6, "a snapshot is one
instant"). Checked against this file directly rather than taken on the
survey's word: `useSidebarNavStore`'s `push`/`pop` drive a resting state
reachable by calling the store directly, exactly the way `sidebar-carousel`
is driven by `setActiveTab`, and every one of `nav-stack.tsx`'s three visual
pictures — the base layer resting, the base layer receded, a pushed screen
showing — is a `transition-[transform,opacity]` **end** state, never a value
gpui would have to animate through. **The reasoning holds without
qualification.**

## 2. An unbounded stack, a bounded contract — the finding worth writing down

`stack.map` renders one `<div>` per pushed screen, however deep the stack
is. `ANCHORS.md` v1.8 refuses two anchors sharing one id, and a synthetic
per-depth id (`nav-stack-screen-0`, `-1`, …) would need an arbitrary cap this
component's own store does not have. Traced whether that cap would cost
anything real, rather than assumed it would:

`web/src/lib/utils.ts`'s `cn = (...i) => twMerge(clsx(i))` resolves
Tailwind's conflicting-utility groups by keeping the **last** class in a
merge group. `-translate-x-1/4` (on every non-top screen, unconditionally)
and `translate-x-full` (appended *only* when `!isTop`) are both members of
the `translate-x` group, so the later `translate-x-full` always wins
outright — every non-top screen's resting class list resolves to
`opacity-0 pointer-events-none translate-x-full`, regardless of depth. A
screen two deep and a screen five deep paint pixel-identical boxes, fully
clipped by the root's own `overflow-hidden` — the same clipping argument
`sidebar_carousel.rs` already made about a snapped-out panel. A second
anchor on a non-top screen could never discriminate a correct port from a
broken one; it would only ever compare a box against its own twin.

**Consequence, on both sides of the port:**

* React: `data-oracle-id={isTop ? 'nav-stack-screen' : undefined}` — at most
  one screen ever carries the id, never zero-or-many by accident.
* Rust: `NavStack` models `top: Option<Screen>`, not a `Vec<Screen>`.

## 3. The base layer's own recede — a margin, not a transform

`sidebar_carousel.rs`'s own module docs record the identical trick for
`scrollLeft`: gpui has no CSS `transform`, so a percentage the DOM resolves
through `translate()` against the element's own border box is reproduced
here as a negative leading margin instead. Numerically the same box here:
the base layer has no authored width, so its own border box and its
containing block's width are the one quantity both `-translate-x-1/4` and
`margin-left: -25%` resolve against — `RECEDE_FRACTION = -0.25`, applied via
`.ml(relative(RECEDE_FRACTION))` only when `top.is_some()`.

## 4. The pushed screen's own header reuses `sidebar_project_header`, not re-derives it

`nav-stack.tsx`'s own comment: "Header — same height and padding as
SidebarProjectHeader." `NavStack::header_height` matches on
[`crowbar_ui::components::keybinding::Platform`] and returns
`sidebar_project_header::HEIGHT_MAC` (44px) / `HEIGHT_OTHER` (34px)
directly — the identical constants, not a second measurement of the same
number. The traffic-light spacer (`w-[72px] shrink-0`, `IS_MAC && !isRight`)
reuses `sidebar_project_header::TRAFFIC_LIGHTS_WIDTH` the same way.

| React / Tailwind | Compiles to | gpui / `crowbar-ui` |
|---|---|---|
| `h-[44px]` (mac) | 44px | `sidebar_project_header::HEIGHT_MAC` |
| `h-[34px]` (other) | 34px | `sidebar_project_header::HEIGHT_OTHER` |
| `w-[72px]` (spacer) | 72px | `sidebar_project_header::TRAFFIC_LIGHTS_WIDTH` |
| `gap-2` (header) | 8px | `HEADER_GAP` |
| `px-3` (header, both edges) | 12px | `HEADER_PADDING_X` |
| `border-b border-border` | 1px, `theme.border` | `HEADER_BORDER_WIDTH` |
| back button: `variant="ghost" size="icon-sm"` | 28×28, `rounded-sm` | `button::Size::IconSm.extent(Breakpoint::Sm)`, `button::RadiusClass::Sm` — reused, not re-derived, the same call `sidebar_project_header.rs`'s own buttons make |
| `<ChevronLeft />` | unpainted | no native equivalent — same call every icon-only button in this port makes |

Unlike `sidebar_project_header`, this header carries **no** `is_right`-driven
asymmetric padding or row-reverse: `px-3` is the same on both edges
regardless of dock side, and the three header children (spacer, back,
title) never reorder. `is_right` here gates exactly one thing — whether the
traffic-light spacer renders at all.

## 5. The title's own line height — left undeclared, and why

`text-[13px] font-semibold text-foreground` carries no paired `line-height`
utility, so its box is CSS `normal` — resolved through the *ambient* font's
own metrics table, not a number Tailwind's compiled CSS states.
`context_pill.rs` had the identical shape (`text-[13px]`, no paired
line-height) and could transfer a known 18px measurement because its own run
shares a font (`font-editor`/`font-mono`, both `var(--editor-font-family)`)
with an already-captured reference. This title has no such donor: it paints
under the *ambient* body font, not `font-mono`, and this item captures no
reference of its own. Declaring `line_sized` here would pin a height this
item cannot honestly derive — `ANCHORS.md` v1.6 warns exactly against
that — so `ID_TITLE` declares neither `content_sized` (the box is
`min-w-0 flex-1`, constrained by flex, not sized to its own content) nor
`line_sized`. A future item with a live capture may find a `bounds.h` delta
on this one anchor; recorded here rather than left to be rediscovered.

## 6. Anchoring

`nav-stack.tsx` carried no `data-oracle-id` before this item. Seven are
added:

* `nav-stack` — the outer wrapper, this surface's own root.
* `nav-stack-base` — the root/children layer, always present.
* `nav-stack-screen` — the pushed screen's own wrapper, **conditional**,
  `isTop` only (§2).
* `nav-stack-header` — the pushed screen's header bar.
* `nav-stack-back` — the header's back button, overriding `button.tsx`'s own
  `'data-oracle-id': 'button'` default, the same namespacing
  `context-pill-trigger`/`sidebar-project-header-*` already establish.
* `nav-stack-title` — the header's title text.
* `nav-stack-body` — the pushed screen's own content area, wrapping the
  opaque `screen.component`.

Composed, not authored here: none — this file paints no other already-ported
primitive.

## 7. Declarations

`CONTENT_SIZED = []`. `LINE_SIZED = []` — see §5 for why the title, despite
being the one true line-sized-shaped box on this surface, is not on the
list.

## 8. The state axis

Every one of the six §8.3 flags is unmodelled — every interactive class in
`nav-stack.tsx`'s own tree lives on the back `<Button>`, which is `button`'s
own surface's business, the identical call `sidebar_project_header.rs`'s
module docs already make about its own four buttons. `--screen`, `--right`,
`--platform`, `--content-width` and `--height` are this surface's own axis
instead. `Params::no_state_axis()` returns `true`.

## 9. `row_layout` coverage

* the resting cell carries exactly the root and the base layer — no
  screen/header/back/title/body anchor exists at all
* `--screen` pushes exactly one screen, carrying every one of its own five
  anchors
* the base layer recedes exactly a quarter of its own width only when a
  screen is pushed, and sits flush at rest — **mutation run**: flipping
  `RECEDE_FRACTION`'s sign turns this red ("expected -73.5px, got 73.5px")
* the pushed screen exactly fills the root regardless of the root's own
  width
* the header's own height follows the platform and matches
  `sidebar_project_header`'s exactly
* the traffic-light spacer needs both mac **and** left-docked — the gap
  between the two dock sides is 80px (72px spacer + the 8px `gap-2` that
  disappears alongside it when the spacer is gone), not 72. **Mutation
  run**: dropping the `!isRight` half of the gate (rendering the spacer on
  every dock side) turns this red at "expected 80px, got 0px" — both
  cells render the spacer, so the two `back` positions coincide
* the content filler moves neither the base layer's own width nor its
  origin
* `--content` reaches the pushed screen's own title text
* `--height` sizes the root's own column

## 10. Reachability

`sidebar-carousel.tsx` → `ide-shell.tsx` → `routes/_shell.tsx`. Confirmed via
`layout-denominator.md` §2 (sole importer: `sidebar-carousel.tsx`) and
`liveness-audit.md`'s own LIVE verdict on `sidebar-carousel` — **not** via
`liveness-audit.md` naming `nav-stack` directly, which it does not (that
audit covers only the 48 already-registered `Surface::names()` entries, a
narrower scope than the 22 remaining Tier B layout targets this file is one
of).
