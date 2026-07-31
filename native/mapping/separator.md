# `separator` (P3.6)

`web/src/components/ui/separator.tsx` →
`crates/crowbar-ui/src/components/separator.rs`.

> A §6.2 row, in the shape `native/MAPPING.md` fixes. Kept in its own file
> because P3 runs four workers in parallel and one appended table is one
> conflict per item.

A thin wrapper over `@base-ui/react`'s `Separator`, which contributes the
`data-orientation` attribute and no styles. Every "Compiles to" below came from
running the app's own `src/index.css` through its own `tailwindcss` 4.3.0 with
the utility as a candidate.

**Reference: none.** See §0 — this is a measurement, not an omission.

## 0. The headline: **there is no reference, and the reason is `:focus`**

`<Separator>` has exactly **two importers** in the whole tree, and both are Plate
editor chrome:

```text
web/src/components/ui/toolbar.tsx       ToolbarGroup's trailing rule
web/src/components/ui/link-toolbar.tsx  the link editor's rules
```

`ToolbarGroup` reaches a screen only through `FloatingToolbar`, and
`floating-toolbar.tsx` gates on Plate's **focused editor**: it reads
`useEventEditorValue('focus')` and hands the result to `useFloatingToolbarState`
as `focusedEditorId`, so the toolbar stays hidden unless the editor under the
caret is the focused one. `link-toolbar.tsx` and `table-node.tsx`'s toolbars sit
*inside* an already-focused editor, so they are strictly harder.

In the automation webview `document.hasFocus()` is **false and immovable** —
measured at the start of the item and again after a full reload. That is the same
measurement already recorded in
`native/oracle/blocked/hover-and-focus-need-an-unlocked-screen.md` as the cause
of every unreachable `:focus` cell. A live count of `[data-slot="separator"]` was
**0** in every state reachable without focus, including with the review pane's
comment surfaces open.

So this is `git-row-dir`'s precedent with the sign flipped: **rendered by the
port, absent from the reachable product.** Nothing in this file is a guess — the
values are the app's own compiled CSS — and what is missing is only the
confirmation a capture would have given.

## 1. Values

| React / Tailwind | Compiles to | gpui / `crowbar-ui` | Oracle |
|---|---|---|---|
| `shrink-0` | `flex-shrink: 0` | `.flex_shrink_0()` | not a field |
| `bg-border` | `background-color: var(--border)` = `oklch(1 0 0 / 6%)` | `theme.color_border` | would be `#ffffff0f` |
| `data-[orientation=horizontal]:h-px` | `height: 1px` — a **literal**, not a spacing step | `THICKNESS` | `bounds.h` |
| `data-[orientation=horizontal]:w-full` | `width: 100%` | `.w_full()` | `bounds.w` |
| `data-[orientation=vertical]:w-px` | `width: 1px` | `THICKNESS` | `bounds.w` |
| `…:self-stretch` | `align-self: stretch` — **conditional**, see §2 | `.self_stretch()` | `bounds.h` |
| — (no radius, no border) | preflight's `border: 0 solid` | | `radius` 0, `border.w` 0 |

Tailwind's `px` scale is a literal `1px` rather than `calc(var(--spacing) * …)`,
so the rule is one logical pixel at every root font size — worth pinning, because
every other size on the neighbouring components *is* a spacing step.

## 2. The trap: `self-stretch` is conditional on the **call site's class**

The vertical arm is not simply `w-px; align-self: stretch`. The full class is

```text
data-[orientation=vertical]:not-[[class^='h-']]:not-[[class*='_h-']]:self-stretch
```

— it stretches **only when the call site names no height of its own**. A
`<Separator orientation="vertical" className="h-4" />` keeps its 4-unit height
and does not stretch.

This is a real branch, not a curiosity. Modelling the stretch as unconditional
would put a full-height rule wherever a call site had asked for a short one, and
the selector is written the way it is precisely because someone expected that to
happen. `CallSite::height()` is the port's spelling of the condition.

**No live call site takes the branch.** All three modelled bundles return `None`,
which is a measurement worth recording: the CSS carries a branch the product
never exercises. `row_layout/separator.rs` asserts every call site stretches, so
the day one grows a `h-*` the assertion says so.

## 3. Declarations

Both lists are **empty**.

* `content_sized` — a separator paints no text, and neither axis is a content
  width: the cross axis is a pinned pixel and the main axis is either `w-full` or
  a stretch, both of which are the *parent's* measure.
* `line_sized` — this box paints no text at all, so there is no line box for a
  height to be derived from. v1.6 makes the declaration valid only on an anchor
  carrying a `font`.

## 4. The surface needs a host, and the host is not an anchor

`self-stretch` resolves against the flex line, so a vertical rule drawn into an
unconstrained container has **no height at all**. `--surface separator` therefore
draws its rule inside a fixed-height row — `HOST_HEIGHT`, 24px, `ToolbarGroup`'s
button row — and that row carries no `data-oracle-id`, so the snapshot holds
exactly one record either way.

The height is the *surface's* number rather than the component's, and that
distinction is the point: nothing in `separator.tsx` names a height, which is
exactly what `self-stretch` means.

§8.3's `empty` is the branch that exercises it: a host with no content leaves the
flex line's cross size at the rule's own `auto`, so a stretched rule collapses to
**zero height** and reports `visible: false`. That is the one property this
component has that a port can get wrong — taffy's `AlignSelf::Stretch` and CSS's
being two implementations of one word — and a port that spelled it as a pinned
height could not collapse. `ToolbarGroup` reaches the same picture from the other
direction: it is `hidden has-[button]:flex`, so an empty group is `display: none`.

## 5. What is **not** modelled

* `link-toolbar.tsx`'s `my-1` and `toolbar.tsx`'s `mx-1.5 py-0.5`: margins on the
  *wrapper*, not on the separator. What they mean here is only that neither class
  starts with `h-`, so the stretch survives them.
* `--content` is **vacuous** — a separator paints no text.
* `--viewport-width` is **vacuous** — `separator.tsx` contains no `sm:` variant.
