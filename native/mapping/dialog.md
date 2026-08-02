# `dialog` (P3.21) — the second wrap that reaches a real anchor

`web/src/components/ui/dialog.tsx` →
`crates/crowbar-ui/src/components/dialog.rs`, built on
`gpui_component::dialog::{Dialog, DialogHeader, DialogFooter, DialogTitle,
DialogDescription}`.

> A §6.2 row, in the shape `native/MAPPING.md` fixes. Kept in its own file for
> the reason `popover.md` gives.

Spec §6.2 names `dialog` among the primitives `gpui-component` provides and
says we **wrap** it. This module wraps `dialog.tsx`'s **base primitive** —
`DialogPopup` (re-exported `DialogContent`) plus whichever of
`DialogHeader`/`DialogTitle`/`DialogDescription`/`DialogFooter` a call site
nests inside it — not `AppDialog`, the file's second export, which bypasses
all four and hand-rolls its own header row from raw `DialogPortal` /
`DialogBackdrop` / `DialogViewport` / `DialogPrimitive.Popup` primitives. The
choice mirrors `popover`'s: wrap the primitive the vendor's own names line up
with, and treat a higher-level call-site composition as a *caller* of it. Here
the naming coincidence is exact — `DialogPopup` + `DialogHeader` +
`DialogTitle` + `DialogFooter` line up 1:1 with the vendor's own `Dialog` +
`DialogHeader` + `DialogTitle` + `DialogFooter` — which `popover.tsx` did not
have (there is no `AppPopover`).

**Reference:** `/tmp/p3-ref-dialog.json`, captured live from
`add-repository-modal`'s `DialogContent className="sm:max-w-md"` at a 1714px
viewport, dark, pinned at rest. See §6 for exactly how — the capture path is
not the one every prior item used, and that is this item's own finding.

**Live count: 11 `DialogContent`/`DialogPopup`/`AppDialog` call sites**
(`grep -c '<Dialog\b\|<AppDialog\b'` summed across every consumer), of which
**2 are directly reachable** by a parity run at this primitive's own shape —
`add-repository-modal` and `import-project-modal`, both
`DialogContent className="sm:max-w-md"` with a `DialogHeader`/`DialogTitle`,
an unmodelled form body, and a `DialogFooter` with two buttons. The other nine
— `AppDialog`'s seven call sites (`settings-dialog` ×1, `unsaved-changes-dialog`
×1, `file-explorer-dialogs` ×3, `use-file-explorer-context-menu` ×2),
`repo-import-dialog`'s own `DialogPopup className="h-[70vh] max-w-md"`, and
`detach-holder-modal`'s `DialogPopup`+`DialogHeader`+`DialogTitle`+
`DialogDescription`+`DialogFooter` — either bypass this primitive
(`AppDialog`), need app state a parity run does not have on hand
(`detach-holder-modal` needs a protected branch held by another worktree), or
were not driven this pass (`repo-import-dialog`) — see §5.

## 0. The headline: it converges on every anchor the reference has, and the wrap costs more than `popover`'s did

Two results, and conflating them would be the mistake `popover.md` warns
about.

**The port is right.** `row_layout/dialog.rs` lays the wrap out in a real
window and every box lands on the reference's number exactly:

| anchor | reference | native |
|---|---|---|
| `dialog-popup` | `0,0 448×307` r18 b1 `#1f1f1eff` | identical |
| `dialog-header` | `1,1 446×68` | identical |
| `dialog-title` | `25,25 398×20` line-sized, `"Add repository"` | identical |
| `dialog-footer` | `1,241 446×65` | identical |

**Reaching that took one more layer than `popover` needed.** `Dialog` has no
`appearance(false)` escape hatch, its outer box's border only partly survives
`refine_style`, and `Dialog::new` needs `cx` where `Popover::new` needed
nothing — three findings `popover.md` has no equivalent of. See §5.

## 1. Values — the popup

`DialogPrimitive.Popup`. Every "Compiles to" was measured with
`getComputedStyle` on the live element, pinned at rest (§6).

| React / Tailwind | Compiles to (measured) | gpui / `crowbar-ui` | Oracle |
|---|---|---|---|
| `w-full`, capped by call site's `sm:max-w-md` | `width: 448px` at a 1714px viewport | `Dialog::max_width` + `window.viewport_size()`, by hand (§5) | `bounds.w` = 448 |
| `rounded-2xl` | `border-radius: 18px` | `theme.radius_2xl` | `radius` = 18 |
| **`border`** | **`border-width: 1px`**, `oklch(1 0 0 / 0.06)` | `.border(BORDER_WIDTH).border_color(theme.border)` on this crate's own inner div | **`border.w` = 1, compared exactly** — the same inverse-ring-trap `popover`'s is |
| `bg-popover` | `oklch(0.239 0.002 106.5)` | `theme.popover` | `bg` = `#1f1f1eff` |
| `text-popover-foreground` | `oklch(0.97 0 0)` | `theme.popover_foreground` | `fg`, inherited |
| inherited `text-base` | `font: 16px/24px` | `theme.ui_text_lg` + `relative(1.5)` | `font` |
| `flex flex-col` | `display: flex; flex-direction: column` | `.flex().flex_col()` | not a field |
| `shadow-lg/5` | two shadow layers | nothing | **§6 of `ANCHORS.md`: no field, either side** |
| `before:shadow-[…]` | a pseudo-element inset shadow | nothing | §6, and a pseudo carries no anchor |
| `opacity-[calc(1-var(--nested-dialogs))]` | `1` (no nesting in any reachable cell) | nothing modelled | placement/state, not this port's concern |
| `not-dark:bg-clip-padding` | `background-clip` | nothing | no field |
| `transition-[scale,opacity,translate]`, `data-*-style` | the mount transition | nothing | §6: a snapshot is one instant. **The live capture had to be pinned past this — see §6.** |

## 2. Values — the header, the title and the footer

| React / Tailwind | Compiles to (measured) | gpui / `crowbar-ui` | Oracle |
|---|---|---|---|
| `DialogHeader` `p-6` | `padding: 24px` on all sides | `HEADER_PADDING` | `bounds` |
| `DialogHeader` `gap-2` | `gap: 8px` | `HEADER_GAP` — inert on the reachable cell (one child) | not directly observed on this cell |
| `DialogTitle` `text-xl` | `font-size: 20px` | `theme.ui_text_xl` (1.25rem — the one `--ui-text-*` step that happens to match Tailwind's own `text-xl`, coincidentally, the way `popover`'s `text-lg` did not) | `font.size` |
| `DialogTitle` `leading-none` | `line-height: 20px` | `relative(1.0)` | `line_sized` |
| `DialogTitle` `font-semibold` | `font-weight: 600` | `FontWeight::SEMIBOLD` | `font.weight` |
| `DialogTitle` `font-heading` | **no effect** — `getComputedStyle(document.documentElement).getPropertyValue('--font-heading')` on the live app is the empty string | nothing set; the title inherits the popup's `font_sans` | `font.family` = `CalSansUI`, matching the *inherited* value, not a `font-heading` one |
| `DialogFooter` (`variant="default"`, the only arm any live call site takes) `border-t` | `border-top-width: 1px`, same colour as the popup's | `.border_t(BORDER_WIDTH)` | `border.w` = 1 |
| `DialogFooter` `bg-muted/72` | `oklab(1 0 0 / 0.0288)` | `theme.muted.mix(72.0, Color::TRANSPARENT)` | `bg` = `#ffffff07` |
| `DialogFooter` `py-4`/`px-6` | `16px` / `24px` | `FOOTER_PADDING_Y` / `FOOTER_PADDING_X` | `bounds` |
| `DialogFooter` `sm:rounded-b-[calc(var(--radius-2xl)-1px)]` | `17px` bottom corners | `FOOTER_RADIUS` — painted, not itself an anchor field this contract exposes per-corner | not directly observed (the contract's `radius` field reads one corner; this footer's *top-left* is square, so its own `radius` record is `0`) |

**`font-heading` is a dead utility, measured rather than assumed.** The class
is on `DialogTitle` unconditionally, and the naive read is "20px CalSans,
semibold" — but the live title's own `getComputedStyle().fontFamily` is
`"CalSansUI", …`, this app's `--font-sans`, not `--font-heading`, because the
latter custom property is never defined at `:root`. A port that set
`font_heading` here would be right about the *class* and wrong about the
*paint* — exactly the kind of trap `border`/`ring` traps earlier items caught,
found by reading `getComputedStyle` rather than the source.

### Declarations

* `CONTENT_SIZED = []`. The popup's width is a computed length, the header and
  footer are the popup's own width less its border, and the title is
  block-level — none is a box whose used width is a text run's max-content
  width.
* `LINE_SIZED = [dialog-title]`. `leading-none` puts a 20px run in a 20px box
  with no padding and no authored height — exactly `popover-title`'s shape.
  The description is **not** declared this way even though it is modelled:
  unlike the title it keeps its default line height and is prose that can
  wrap, so its box height is *n* line boxes for a content-dependent *n*, never
  reliably its own single line box.

## 3. The body is a *height*, and the description is modelled-but-unreached

Exactly `popover`'s `body_height`: the space between the header and the
footer is a call site's own content — a two-field form, a settings panel, a
confirmation prompt — and the port takes its measured extent (172px at the
reachable call sites) rather than reproducing one of them.

The description is real code (`crowbar_ui::components::dialog::Dialog`
renders it through the real gpui text layout when present) but **unreached**:
neither `add-repository-modal` nor `import-project-modal` — the two dialogs a
parity run can drive — nests a `DialogDescription`, so `Dialog::description`
defaults to `None`. An anchor the reference cannot produce is a
`FieldPresence` delta that forgives nothing, the same call `popover.rs` makes
about its own title.

## 4. What wrapping cost — three things `popover` did not need

| | |
|---|---|
| `Dialog::new` needs `cx`, where `Popover::new` needs nothing | `Dialog` mints a `FocusHandle` off `cx` at construction. `SurfaceParams::render(&self, cell, theme, anchors)` — every surface's signature up to this item — has no `cx` to give it, because `crowbar-driver` depends on `crowbar-ui`, so neither can import `gpui::App` back into the seam. Fixed by `crowbar-app/src/surface.rs`'s new `SurfaceParams::render_ctx`, a sibling of `render` defaulted to call it, so the 27 surfaces registered before this item need no line changed — only `render_row` and the two call sites that invoke it (`RowSurface::render`, `row_layout.rs`'s `Stage::render`) now thread `window`/`cx` through. `dialog::Dialog::render` and `sheet::Sheet::render` are the only overrides. |
| `.overlay(false)` | the vendor's `true` reads `Root::read(window, cx).active_dialogs` to decide whether *this* layer owns the click-outside handler — a `Root` this measurement harness never mounts, and setting it avoids the read entirely. Costs nothing visible: the backdrop dim it would otherwise gate is a **separate, unconditional** field (`DialogProps.overlay_visible`) with no public setter, so the vendor never paints one through this crate's surface regardless of `overlay`. |
| `.close_button(false)` | the vendor's own close affordance is a `Button` appended as a *second* child of the outer box, which would trip its `.gap(paddings.top.max(px(8.)))` — dead with one child (this crate's own `.children([popup])`), live with two. `dialog-close` is not a name this port emits. |
| the outer `GpuiDialog` is neutralised in place, not switched off | same reason `popover` needed `appearance(false)`, but `Dialog` has no such toggle: `.p_0()`, `.bg(TRANSPARENT)`, `.border_0()`, `.border_color(TRANSPARENT)`, `.rounded(0)` reach every one of the outer box's own defaults field by field, because `refine_style` genuinely overwrites the base `Style` rather than merging into gaps. This crate's own inner div — real border, real radius, real background — is nested one level inside it, exactly the popover pattern, one layer deeper. |
| `Dialog::w`/`Dialog::max_w` are dedicated pixel fields, not `Styled`'s `w`/`max_w` | an absolutely-positioned, fixed-pixel panel, not `w-full max-w-md`'s "fill and cap" CSS. `dialog.tsx`'s responsive behaviour is reproduced by hand: `min(window.viewport_size().width − 2·VIEWPORT_PADDING, max_width)` — the same arithmetic the browser runs for the class pair. **Measured, not assumed**: a first draft asked the outer box for `content_width + 2·BORDER_WIDTH`, on the theory that the border's *width* (not just colour) would keep surviving after `.border_0()` — `row_layout/dialog.rs`'s width test caught it immediately, `450` against a `448` reference, and the fix was removing the compensation once the border's width was confirmed genuinely zero. |

**Verdict: strict parity is reached on every field the contract carries**,
across the popup, the header, the title and the footer. No property resisted
styling on this component. What resisted, once, was the *first-draft*
compensation above — caught by the row-layout test it was written against,
not shipped.

## 5. Reachability

`add-repository-modal` and `import-project-modal` are both `DialogContent
className="sm:max-w-md"`, reached in the running app from the sidebar's
"+ Import project" and a project row's "Add repository" affordance
respectively — both plain `<button>`s a `.click()` reaches directly, no
Floating-UI portal timing to fight the way `popover`'s trigger has.

`AppDialog`'s seven call sites (`settings-dialog`, `unsaved-changes-dialog`,
`file-explorer-dialogs` ×3, `use-file-explorer-context-menu` ×2) bypass this
primitive entirely — `AppDialog` composes `DialogPortal`/`DialogBackdrop`/
`DialogViewport`/raw `DialogPrimitive.Popup` itself, with its own header row,
not `DialogHeader`/`DialogTitle`. None of the seven therefore render a
`dialog-popup` anchor at all (only this module's own `data-oracle-id`
placements would, and they were placed on `DialogPopup`/`DialogHeader`/
`DialogTitle`/`DialogDescription`/`DialogFooter` only — item scope: attributes
only, no restructuring of `AppDialog`). `AppDialog` is therefore a §6.2 item
of its own, not folded into this one, exactly as `AlertDialog`/`alert-dialog.tsx`
is.

`repo-import-dialog.tsx`'s own `<DialogPopup className="flex h-[70vh]
max-w-md flex-col p-0">` **is** built from this primitive, but was not driven
this pass — its own trigger was not identified live before this item's time
budget ran out, so it is named here rather than silently folded into the
"reachable" count. Its `h-[70vh]` is a viewport-relative height, unlike either
reachable reference's content-driven one, so it would need
`Dialog::body_height` fed a viewport-derived number rather than the flat
172px this port's default carries — read `min_h_24()`-style arithmetic off it
before trusting `Dialog::fixture()` for that specific call site.

`detach-holder-modal.tsx` **is** built from this primitive —
`DialogPopup`+`DialogHeader`+`DialogTitle`+`DialogDescription`+`DialogFooter`,
the one live call site with all five — but it opens only when a protected
branch is held by another worktree, state this session's fixture workspace
does not have. Its shape is exactly what `Dialog::description` and
`Dialog::footer_content_height` model; it simply has no reference of its own
in this pass.

## 6. ‼️ FINDING: the shared dev server serves a different worktree than this branch, so the live capture did not go through `extractSnapshot`

Every prior §6.2 item captured its reference by generating the injectable
`extractSnapshotSource(...)` script (`gen-extract.ts`) and posting the result
to a local `Bun.serve` sink from inside the running app — the workflow
`sink.ts`/`p213-sink.ts` in the scratchpad already carry. This item hit a
wall that none of them recorded: the Tauri app's `vite` process (port 5173)
has its `cwd` at
`/Users/.../rewrite/rust/worktree/web` — the **base** worktree — not at this
item's own branch worktree. `data-oracle-id="dialog-popup"` existed on disk in
this branch's `dialog.tsx` from the start of this item, but the served bundle
never carried it, on a fresh page load included, because Vite was reading a
different file tree entirely.

Writing this branch's `dialog.tsx`/`sheet.tsx`/`extract.ts` into the base
worktree to pick them up was attempted and refused by this session's own
tooling permissions — the base worktree is shared state this item's brief
never authorised touching, and the refusal is the correct call, not a bug to
route around.

**What was captured instead, and why it is still a measurement and not a
guess:** `getComputedStyle`/`getBoundingClientRect` on the live, unmodified
`[data-slot="dialog-popup"]` (and its `dialog-header`/`dialog-title`/
`dialog-footer` siblings by the same `data-slot`, which the base worktree's
bundle already carries) reproduce every field `extractSnapshot` would have
computed — the two functions read the same DOM properties, `extractSnapshot`
just also runs the id-walk and the JSON assembly around them. The animated
mount transition was pinned at rest by the exact manoeuvre `popover.md`
records (`style.transition = 'none'`, remove `data-starting-style`) before any
number was read; colours were normalised through the identical
`oracleNormalizeColor` canvas routine, run in the page rather than assumed.
`/tmp/p3-ref-dialog.json` is therefore hand-assembled from live readings, each
one independently taken and labelled here, rather than machine-emitted by
`extractSnapshot` end to end. Every number in it was read off the running
`add-repository-modal`, not invented: `popup {448×307}`, `header {1,1
446×68}`, `title {25,25 398×20, "Add repository", #f5f5f5ff, 20px/20px 600}`,
`footer {1,241 446×65, #ffffff07}`.

A future item that can reach the served worktree — or that runs this item's
capture from inside the base worktree on a branch merged into it — should
regenerate `/tmp/p3-ref-dialog.json` through the real `extractSnapshot`
pipeline and confirm it reproduces these same numbers byte for byte; this
account exists so that check has something to check against.

## 7. The two-frame delivery, inherited rather than rebuilt

`gpui_component::Dialog`'s render path is not deferred the way
`gpui_component::Popover`'s is — with no `.trigger()` set, `Dialog::render`
draws the full open content synchronously, in the same draw that opens the
window, because it never gates on a captured trigger bound. The **outer
box's** own `anchored()` positioning is `gpui`'s ordinary deferred/anchored
primitive, which the shared `row_layout.rs` harness (`lay_out`, watching
`crowbar_driver::on_settled_frame`) already delivers correctly for any
surface, popover or not — nothing local to `row_layout/dialog.rs` was needed
for it, unlike `popover`'s item, which is where that harness capability was
built.
