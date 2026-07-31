# `sidebar-toggle-icon` (P3.8)

`web/src/components/ui/sidebar-toggle-icon.tsx` →
`crates/crowbar-ui/src/components/sidebar_toggle_icon.rs`.

> A §6.2 row, in the shape `native/MAPPING.md` fixes. Its own file, per the P3
> process note.

One `<svg>` with a `24 × 24` viewBox: a rounded `<rect>`, a divider `<path>` and
two rail `<path>`s, all `fill="none"` and stroked 2px in `currentColor`. Its
class list is `cn('size-4', className)` and nothing else.

**Reference:** `/tmp/p3-ref-sidebar-toggle-icon.json`, captured live from the
sidebar header's toggle at a 1714px viewport.

## 0. The headline: `size-4` is an **opt-out**, and it is the whole design

`native/MAPPING.md` records that `button`'s base class list carries

```text
[&_svg:not([class*='size-'])]:size-4.5  sm:[&_svg:not([class*='size-'])]:size-4
```

and that this beats a presentational `width`/`height` attribute outright — P3.2
measured a phosphor `size={14}` rendering at 16, because presentation attributes
have no specificity.

**This icon escapes that rule by carrying a `size-` class of its own**, and the
primitive's source comment says that is exactly what it is for: without it the
glyph took whichever size its button variant dictated, and one component rendered
at four sizes in four places.

The escape is **total rather than conditional**, which is the part worth writing
down. `cn` is tailwind-merge: a call site's own `size-*` *replaces* `size-4` and
still matches `[class*='size-']`; a call site that names no size leaves `size-4`
in place. **There is no class list a call site can pass that lets the button's
rule apply.** So `--viewport-width` is vacuous on this surface where it is real
on `search-toggle-icons` next door, and the two are each other's control:
`crates/crowbar-app/src/row_layout/` asserts 16 at both breakpoints here and
18 → 16 there.

## 1. Values

| React / Tailwind | Compiles to | gpui / `crowbar-ui` | Oracle |
|---|---|---|---|
| `size-4` | `width: calc(var(--spacing) * 4)` = **16px**, same for `height` | `EXTENT` | `bounds.w`, `bounds.h` = 16 |
| `viewBox="0 0 24 24"` | not CSS | `VIEW_BOX` | **invisible** |
| `fill="none"` | not a contract field | not drawn | **invisible** |
| `stroke="currentColor"` | resolves the button's `color` | not drawn | **invisible** — `fg` needs a text node |
| `strokeWidth={2}` | `stroke-width: 2` | not drawn | **invisible** |
| `strokeLinecap`/`strokeLinejoin` `round` | — | not drawn | **invisible** |
| `<rect x=3 y=4 w=18 h=16 rx=2.5>` | viewBox geometry | not drawn | **invisible — and see §2** |
| `<path d="M9 4v16">` (divider) | viewBox geometry | not drawn | **invisible** |
| `<path d="M5.5 9h1.5">`, `M5.5 13h1.5` (rails) | viewBox geometry | not drawn | **invisible** |
| *(nothing)* | preflight's `border: 0 solid` | — | `border.w` = 0, colour `#ffffff0f` never painted |
| *(nothing)* | no `background`, no `border-radius` | — | `bg` `#00000000`, `radius` 0 |

## 2. ⚠ `rx="2.5"` is **not** the anchor's `radius`

The one way this component's invisibility can bite in the *opposite* direction.
`radius` reads the element's CSS `border-radius`, which is **0** here; the rect's
`rx` is geometry inside the viewBox and reaches no field. A port that translated
it into `.rounded(px(2.5))` would paint a real corner on the box, and the differ
*would* call that — a delta on a component whose art is otherwise unmeasurable.
`crates/crowbar-app/src/row_layout/sidebar_toggle_icon.rs` asserts `radius` off
the recorded anchor rather than off a constant, so a stray `.rounded(…)` fails
there.

## 3. Declarations

| | Value | Why |
|---|---|---|
| `content_sized` | **false** | `size-4` **authors** the width |
| `line_sized` | **false** | `size-4` authors the height too, and the glyph paints no text — `ANCHORS.md` v1.6 needs a `font` |

## 4. Reachability — **1 live instance of 2 call sites**

| Call site | Live count | Notes |
|---|---|---|
| `components/layout/sidebar-project-header.tsx` | **1** | the sidebar's leading-edge toggle — the captured cell |
| `features/tabs/components/tab-navigation-buttons.tsx` | **0** while the sidebar is open | it renders the toggle *in the tab bar* when the sidebar is hidden; the two swap places |

Both wrap the glyph in `Button variant="ghost" size="icon-sm"` with a
`shrink-0 rounded-sm text-muted-foreground hover:bg-sidebar-element-hover`
className **on the button**, and **neither passes a className to the icon**.
That is why `CallSite::override_extent` is `None` at every one of them — the
branch exists in the button's CSS and nothing in the app takes it, which is
`separator::CallSite::height`'s situation exactly.

## 5. What is **not** modelled

* **The glyph.** Stroke, path data, line caps: none has a field.
* **The button around it.** `hover:bg-sidebar-element-hover` moves the *button's*
  background, which is `button`'s anchor on `button`'s surface. Nothing on the
  icon responds to any interaction, which is why five of six §8.3 flags are
  unmodelled here.
* **`--theme`, `--content`, `--width`, `--viewport-width`** — all vacuous. The
  stroke colour is the only theme-varying thing on the element and it reaches the
  contract through no field.
