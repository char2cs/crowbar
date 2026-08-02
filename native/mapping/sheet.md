# `sheet` (P3.21) — the wrap with no live call site at all

`web/src/components/ui/sheet.tsx` →
`crates/crowbar-ui/src/components/sheet.rs`, built on
`gpui_component::sheet::Sheet` +
`gpui_component::dialog::{DialogHeader, DialogTitle, DialogDescription}`
translated by hand (`sheet.tsx`'s `SheetHeader`/`SheetTitle`/
`SheetDescription` reuse `dialog.tsx`'s exact class strings, so this module's
`header`/`title_box`/`description_box` are copied from `dialog::Dialog`'s,
not a second design).

**Reference: none.** See §1 — this is the item's finding, not an omission.

**Live count: zero.**

## 1. ‼️ FINDING: the one importer never mounts it

`sheet.tsx`'s only consumer anywhere in `web/src` is `components/ui/sidebar.tsx`'s
`Sidebar`, and only in its `if (isMobile)` branch:

```tsx
if (isMobile) {
  return (
    <Sheet onOpenChange={setOpenMobile} open={openMobile} {...props}>
      <SheetPopup ...>
```

`grep -rn '<Sidebar\b' web/src` finds nothing outside `sidebar.tsx` itself and
its own test file. Every feature file that imports from `@/components/ui/sidebar`
(`ide-shell.tsx`, `tab-bar.tsx`, `pane-container.tsx`,
`file-explorer-tree.tsx`, `sidebar-project-header.tsx`) imports
`SidebarProvider` and/or the `useSidebar()` hook — for Crowbar's own
scroll-snap carousel sidebar's collapse state — and never the `Sidebar`
component that would actually render a `Sheet`.

**Verified live, not inferred from the grep alone.** The running app's window
was resized to 700 logical px — comfortably under Tailwind's 768px `md`
breakpoint, which is what `useMediaQuery('max-md')` (`Sidebar`'s own
`isMobile`) reads — and nothing on screen changed shape into an off-canvas
sheet, because there is no `Sidebar` mounted anywhere to switch into its
mobile rendering. The window was restored to 1714px afterward.

This module is written and tested anyway, per the brief: "if a component is
not reachable, port it and say so — do not fabricate a reference." No
`native/oracle/runs/` entry exists for it and none should: there is nothing on
the React side to converge against, so a `/tmp/p3-ref-sheet.json` would be a
number with nothing behind it.

## 2. Values, spelled the same way as `dialog`'s but with no live check

| React / Tailwind | Compiles to (by the same arithmetic `dialog`'s uses) | gpui / `crowbar-ui` |
|---|---|---|
| `bg-popover` | `oklch(0.239 0.002 106.5)` | `theme.popover` |
| `text-popover-foreground` | `oklch(0.97 0 0)` | `theme.popover_foreground` |
| bare `border-s`/`border-e`/`border-t`/`border-b` (one side, by `side`) | `1px`, same colour as `dialog`'s border | `BORDER_WIDTH` + `theme.border` |
| `SheetHeader` `p-6`/`gap-2` | `24px`/`8px` | `HEADER_PADDING`/`HEADER_GAP` — copied from `dialog`'s |
| `SheetTitle` `text-xl leading-none font-semibold` | `20px/20px, 600` | copied from `dialog::Dialog::title_box` |
| `SheetTitle` `font-heading` | dead, same finding as `dialog`'s — `--font-heading` is undefined at `:root` | not set; inherits `font_sans` |
| `SheetDescription` `text-muted-foreground text-sm` | `14px`, default line height | copied from `dialog::Dialog::description_box` |
| `w-[calc(100%-(--spacing(12)))] max-w-md` (the panel's main axis, `Right`) | `min(viewport − 48, 448)` | `EDGE_MARGIN` / `MAX_SIZE`, the same two-armed formula `dialog`'s `max_width` uses |

`sheet.tsx` has **no `SheetFooter`** — `grep -c SheetFooter sheet.tsx` is `0`
— so unlike `dialog`, this module models no footer at all; `Sheet` carries
only a popup, an optional header, a title and a description.

### Declarations

* `CONTENT_SIZED = []`, for the same reasons `dialog`'s is.
* `LINE_SIZED = [sheet-title]`, for the same reason `dialog-title`'s is —
  `leading-none` on a 20px run.

## 3. What resisted, precisely — four points, three more than `dialog` hit

`dialog.md` §4 lists three costs `popover` did not have. `sheet` has all
three (`cx` at construction, an outer box that must be neutralised in place,
`w`/`max_w` as dedicated fields rather than `Styled`) and, being a strictly
worse-behaved widget to wrap, one further one:

1. **The border width is not reachable through `refine_style` at all.**
   `Dialog`'s `.border_1()` runs *before* `refine_style`, so `.border_0()`
   overwrites it — the mechanism `dialog.md` §4 documents. `Sheet`'s per-side
   border (`.border_l_1()` / `.border_r_1()` / `.border_t_1()` /
   `.border_b_1()`) is chosen *inside* a `.map(|this| match self.placement {
   … })` that runs **after** `refine_style`, so nothing this crate sets
   survives it. Measured, not assumed: `row_layout/sheet.rs`'s width test was
   first written without any compensation and read `447` against this crate's
   own `448` claim — one pixel short on the bordered side, the *asymmetric*
   twin of `dialog`'s two-pixel (both sides) finding. The fix is the same
   shape: `.size(content + BORDER_WIDTH)` on the main axis, so the surviving
   1px border eats exactly the pixel it was always going to eat and the
   *content* box comes out at the declared number.
2. **`Sheet.placement` is `pub(crate)`, with no public setter reachable
   without a mounted `Root`.** `WindowExt::open_sheet_at(placement, cx,
   build)` is the *only* public way to choose a placement, and it stores the
   sheet on `Root::active_sheet` — the same `Root` dependency `dialog::Dialog`
   avoids entirely by never calling `.trigger()`. Constructing `GpuiSheet::new`
   directly, as this module does (to stay `Root`-free, matching `dialog`'s own
   choice and this measurement harness's own constraint — no `Root` is ever
   mounted), always gets `Sheet::new`'s own hard-coded default:
   **`Placement::Right`**, unconditionally. `sheet.tsx`'s `Left`/`Top`/`Bottom`
   classes are read and translated in this module's docs (§2's cross-axis
   note) but are not reachable in `render`'s actual output. Left as a
   documented limitation rather than a `Root`-mounting workaround, because the
   only two known callers of `open_sheet_at` in this codebase are inside
   `gpui-component` itself.
3. **The vendor renders a title bar this crate cannot suppress.** Unlike
   `Dialog::close_button(bool)`, `Sheet` has no method that removes its own
   `h_flex().justify_between()` row — a close `gpui_component::Button` beside
   whatever `.title()` was given, *unconditionally*, above whatever this
   crate's own `.children()` supplies. `sheet.tsx` has no such row at all:
   `SheetHeader` is an entirely optional slot a call site places among its own
   children, and the close affordance is a `SheetPrimitive.Close` floated
   `absolute end-2 top-2`, not a title-bar button. There is no configuration
   that reaches parity here — the vendor's title bar paints regardless, above
   this module's own `sheet-header`, not in its place. Invisible to the oracle
   today only because there is nothing on the other side to compare it
   against.
4. **The panel's cross axis is not content-driven.** `Sheet::render` positions
   the panel `top(margin_top)…bottom_0()` for `Right` (and `Left`/`Top`) — an
   absolute box between two fixed window edges, not `auto`-height — where
   `sheet.tsx`'s own `side="right"`/`"left"` sheets have no top offset at all
   (full window height). `Sheet::body_height` is still rendered as this
   module's own unanchored box, exactly as `dialog`'s body is, but it does not
   drive `sheet-popup`'s own height the way `dialog-popup`'s does — the
   outer's height comes from the vendor's placement positioning regardless of
   content. A fact about this placement, not a defect in this module: there is
   no reference to be wrong against yet.

None of the four costs anything measurable today, because there is nothing on
the React side to measure against — but a future item that finds a live sheet,
or a way to reach `Root` from this measurement harness, should read this list
before trusting the port past `Placement::Right`.

## 4. Why this item still built it

The brief: "measure reachability and report the live count… if a component is
not reachable, port it and say so." `sheet` is on §6.2's named wrap list
beside `dialog`, and the two share almost their entire header/title/
description shape (`sheet.tsx` literally reuses `dialog.tsx`'s class strings
for all three) — porting it alongside `dialog` costs one file and a handful of
tests, not a second design, and it means the day a `Sidebar` call site *is*
added, this module is already there to be pointed at it rather than a second
`native/p3.2x-wrap-sheet` item having to rediscover everything §3 above
records about the vendor's shape.
