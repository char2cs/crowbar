# `popover` (P3.15) — the first **wrapped** component

`web/src/components/ui/popover.tsx` →
`crates/crowbar-ui/src/components/popover.rs`, built on
`gpui_component::popover::Popover`.

> A §6.2 row, in the shape `native/MAPPING.md` fixes. Kept in its own file
> because P3 runs several workers in parallel and one appended table is one
> conflict per item.

Spec §6.2 names `popover` among the primitives `gpui-component` provides and
says we **wrap** it. This is the first component in the tree that does, so this
file records the division of labour and the price, not only the values —
eight more wrapped components will lean on it.

**Reference:** `/tmp/p3-ref-popover.json`, captured live from the sidebar's
`RepoIconPopover` at a 1714px viewport, dark, with the popup pinned at rest.

**Live count: 4 `PopoverContent` call sites**, of which **1 is reachable** by a
parity run: `repo-icon-popover` (sidebar). `commit-popover` and `merge-popover`
both sit behind a git panel that needs a dirty worktree; `equation-node` is
inside the Plate editor, which §3.2 puts out of scope.

## 0. The headline: it converges, and it cannot be captured

Two separate results, and conflating them would be the mistake.

**The port is right.** `row_layout/popover.rs` lays the wrap out in a real
window and every box lands on the reference's number exactly:

| anchor | reference | native |
|---|---|---|
| `popover-popup` | `0,0 256×177` r10 b1 `#1f1f1eff` | identical |
| `popover-viewport` | `1,1 254×175` | identical |

**The driver cannot emit it.** Measured, not inferred:

```
$ CROWBAR_ROW_SNAPSHOT=… crowbar-app --surface popover
crowbar-app: no snapshot: the root anchor "popover-popup" was not recorded
             this frame; the anchors that were: []
```

See §4. This is the item's finding and it is **not specific to `popover`**.

## 1. Values — the popup

`PopoverPrimitive.Popup`. Every "Compiles to" was measured with
`getComputedStyle` on the live element.

| React / Tailwind | Compiles to (measured) | gpui / `crowbar-ui` | Oracle |
|---|---|---|---|
| `w-(--popup-width,auto)`, call site `w-64` | `width: 256px` | `Popover::width` | `bounds.w` = 256 |
| `rounded-lg` | `border-radius: 10px` | `theme.radius_lg` | `radius` = 10 |
| **`border`** | **`border-width: 1px`**, `oklch(1 0 0 / 0.06)` | `.border(BORDER_WIDTH).border_color(theme.border)` | **`border.w` = 1, compared exactly** |
| `bg-popover` | `oklch(0.239 0.002 106.5)` | `theme.popover` | `bg` = `#1f1f1eff` |
| `text-popover-foreground` | `oklch(0.97 0 0)` | `theme.popover_foreground` | `fg`, inherited |
| inherited `text-base` | `font: 16px/24px` | `theme.ui_text_lg` + `relative(1.5)` | `font` |
| `flex` | `display: flex` | `.flex()` | not a field |
| `shadow-lg/5` | two shadow layers | `.shadow_lg()` | **§6: no field, either side** |
| `before:shadow-[0_1px_…]` | a pseudo-element inset shadow | **nothing** | §6, and a pseudo carries no anchor |
| `not-dark:bg-clip-padding` | `background-clip` | **nothing** | no field |
| `transition-[width,height,scale,opacity]` | — | nothing | §6: a snapshot is one instant |
| `data-starting-style:scale-98 opacity-0` | the mount transition | nothing | see §3 |
| `origin-(--transform-origin)` | positioner output | nothing | placement |

**The border is the inverse of `dropdown_menu`'s ring trap.** There, `ring-1`
compiles to a box-shadow and `border.w` is **0** on both sides; a port reaching
for `.border_1()` would report 1 against 0 on every cell. Here the class is a
bare `border`, which really is 1px of border, and a port that "learned" the ring
lesson and left the border off would be wrong in the other direction by exactly
the same amount. **Measure; never infer** — the brief's warning, and this pair
is the proof that it runs both ways.

## 2. Values — the viewport and the title

| React / Tailwind | Compiles to (measured) | gpui / `crowbar-ui` | Oracle |
|---|---|---|---|
| `size-full` | `254 × 175` | `.w_full()` + flex stretch | `bounds` |
| `py-4` | `padding-block: 16px` | `VIEWPORT_PADDING` | `bounds.h` |
| `px-(--viewport-inline-padding)`, `[…:--spacing(4)]` | `padding-inline: 16px` | `VIEWPORT_PADDING` | `bounds.w` of children |
| `overflow-clip` + `not-data-transitioning:overflow-y-auto` | `overflow: hidden auto` | `.overflow_hidden()` | drives `clipped` |
| `relative` | `position: relative` | `.relative()` | not a field |
| `max-h-(--available-height)` | positioner output | **nothing** | placement — as on `dropdown-menu` |
| `PopoverTitle` `text-lg` | `font-size: 18px` | `rems(1.125)` | `font.size` |
| `leading-none` | `line-height: 18px` | `relative(1.0)` | `line_sized` |
| `font-semibold` | `font-weight: 600` | `FontWeight::SEMIBOLD` | `font.weight` |

**`text-lg` has no token.** `--ui-text-lg` is `1rem` and `--ui-text-xl` is
`1.25rem`; this design system carries no member of Tailwind's scale at
`1.125rem`. Spelled as a bare `rems(1.125)`, which is the trade `dropdown_menu`
makes for `text-xs`, one step further along — minting a token would put a value
in the system the system does not have.

### Declarations

* `CONTENT_SIZED = []`. The popup's width is a definite length, the viewport is
  `size-full` inside it, and the title is a block-level `<h2>` — none of the
  three is a box whose used width is a text run's max-content width.
* `LINE_SIZED = [popover-title]`. `leading-none` puts an 18px run in an 18px box
  with no padding and no authored height, which is exactly v1.6's shape. It
  costs nothing here — 18 is whole, so `floor` and `pixel_snap` agree — and is
  declared because it is **true**, which is the test v1.6 sets.

## 3. The body is a *height*, and why that is not a fudge

`PopoverContent`'s children are the call site's, and every live one is a
different sub-UI. The port takes their measured extent (`--body-height`, 143px
at the reachable call site) exactly as `dropdown-menu` takes the positioner's
`--anchor-width`: a runtime quantity the primitive does not decide, declared
rather than computed, so that the two boxes which genuinely **are** this
primitive compare against a reference whose content is whatever it happens to
be. `row_layout` pins the relationship at three body heights and three widths.

## 4. ‼️ THE FINDING: `gpui-component`'s popup needs **two frames**, and the emit path gives it one

`gpui_component::Popover::render`:

```rust
if !open || !trigger_bounds_captured { return el; }   // frame 1 stops here
```

`trigger_bounds_captured` is set in the **trigger's `on_prepaint`**, which runs
*after* `RenderOnce::render` has already returned. So frame 1 is the trigger and
nothing else, whatever `open` says; the vendor then calls
`window.request_animation_frame()` and frame 2 has the popup.

`Window::request_animation_frame` is `on_next_frame(|_, cx| cx.notify(…))`. Both
places that consume the popup deliver that one frame too late:

| | mechanism | result |
|---|---|---|
| `crowbar-app --features driver` | `main.rs` emits from `window.on_next_frame`, and gpui runs **all** next-frame callbacks at the *top* of a frame request, before `window.draw`. The emit callback is registered first (at `open_window`) and calls `cx.quit()`. | registry holds frame 1 → **0 anchors** → `MissingRoot` |
| `row_layout`'s shared `measure` | `open_window` + `run_until_parked`. gpui's own comment on `simulate_next_frame` says why: *"Tests have no platform frame loop"* — `run_until_parked` drives the executor, not the frame loop. | **0 anchors** (measured) |

`row_layout/popover.rs` carries a **local** harness that calls
`Window::simulate_next_frame` by hand, and asserts that the vendor really did
ask for a frame — so the port is measured, and the second frame is visible in
one file rather than hidden in the shared one. The driver has no such escape:
the fix is in `main.rs`/`surface.rs`, which are shared, and is the
orchestrator's call.

**This generalises to most of the §6.2 list.** Every `gpui-component` widget
whose content is `deferred(anchored(…))` behind a captured trigger bound has the
same shape — `dialog`, `sheet`, `combobox`, `menu`, `context_menu`, `tooltip`,
`hover_card`. `select` has the same root cause in a nastier form: its popup
*does* render on frame 1, but off `SelectState.bounds`, which is still
`Bounds::default()` — so the menu is **2px wide** rather than absent, and a
one-frame capture would look like a port defect instead of a missing frame.

## 5. What wrapping cost

| | |
|---|---|
| `appearance(false)` | the vendor's `true` paints `popover_style(cx)` — **`gpui-component`'s theme, not ours** — plus a `p_3()` this primitive does not have. |
| the popup box is **ours** | the vendor builds its content box inside the private `render_popover_content`. `AnchorSink` takes a `Div` this crate *holds*, so a box we never hold cannot carry an anchor. With `appearance(false)` the vendor's box paints nothing and ours is the visible popup. |
| `ParentElement`, not `content` | `Popover::content` needs `F: … + 'static`; a component here holds `&dyn AnchorSink` with a lifetime. Children are already-built `AnyElement`s and land in the same box. |
| `Trigger` | `Popover::trigger` needs `T: Selectable`, `gpui-component`'s own trait, which no gpui element implements. `PopoverTrigger` has no class list of its own, so the trigger here is an unstyled box that satisfies the bound. |
| `gpui_component::init` | the wrap reaches `GlobalState` on the way to opening. `main.rs` calls it; the shared test harness does not, so this surface's tests call it themselves. |

**Layers the wrap adds that base-ui does not:** a `v_flex().id("content")
.occlude().tab_group()` with a `top_1()`. It paints nothing, and `row_layout`
asserts it neither stretches nor offsets the popup — that `v_flex`'s cross-axis
default is `stretch`, so a popup without its own width would have been pulled to
it.

**Verdict: strict parity is reached on every field the contract carries.** No
property resisted styling. What resisted is *capture*, and that is §4.

## 6. Reachability, and how the reference was taken

`document.hasFocus()` is false and `document.visibilityState` is **`hidden`**,
so rAF never fires and base-ui never removes `data-starting-style` — the popup
sits at `scale: 0.98; opacity: 0` indefinitely. `w.show()`, `w.setFocus()` and
an AppleScript `frontmost` all failed to change it.

The capture therefore **pins the mount transition at rest**
(`style.transition = 'none'`, then remove `data-starting-style`), which is the
direct analogue of the `animation.pause(); currentTime = 0` that `ANCHORS.md`
v1.9 prescribes for an animated reference: it advances the component to the
instant it reaches on its own, and adds nothing. Unpinned it measures
`250.88 × 173.46` — 0.98 of the truth — which is exactly the "wrong nearly
always" v1.9 warns about.

**The reference was reduced by hand**, and the reduction is declared: the
capture under `popover-popup` also contained `avatar`, `avatar-fallback` and
**`button` ×3** from the `repo-icon-popover` call site. `popover` is not in
`extract.ts`'s `oracleSurfaceScope` table, and this item's brief scopes the React
edit to `data-oracle-*` attributes and nothing else, so the surface cannot
declare its set. The raw capture is `/tmp/p3-raw-popover.json`; the reference
keeps the two `popover-*` ids. This is the same workaround `sidebar-carousel`
took, it works for the same reason (the ids are unique), and it wants the same
fix: an entry in `oracleSurfaceScope`, which is a one-line change nobody on this
item was scoped to make.
