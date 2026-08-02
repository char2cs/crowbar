# `command` (P3.32) — a second hand-rolled dialog shell, not a caller of `dialog.rs`

`web/src/components/ui/command.tsx` →
`crates/crowbar-ui/src/components/command.rs`, wrapping
`gpui_component::dialog::Dialog` for its own shell and reusing
`crates/crowbar-ui/src/components/autocomplete.rs`'s `Input`/`List`/`Item`/
`ListContent` for everything else.

> A §6.2 row, in the shape `native/MAPPING.md` fixes. Ported together with
> `autocomplete` — see `autocomplete.md` — because `command.tsx` is
> `autocomplete.tsx`'s one importer.

**Reference:** `/tmp/p3-ref-command.json`, captured live from the workspace
switcher (`web/src/components/layout/workspace-switcher.tsx`, opened through
`context-pill.tsx`'s `CommandDialog`), at a 1714px viewport, dark, pinned at
rest. Root `command-dialog-popup`.

**Live count: 2 consumer files** (`code-block-node.tsx`'s own comment names
this file but imports `cmdk`/`@radix-ui/react-popover` directly instead — see
§5 — so it is not a third). `workspace-switcher.tsx` (`Command`/
`CommandInput`/`CommandPanel`/`CommandList`/`CommandItem`/`CommandFooter`) and
`context-pill.tsx` (`CommandDialog`/`CommandDialogTrigger`/
`CommandDialogPopup`) compose to the **one** reachable command palette: the
Context Pill's "Switch workspace" button, a plain `<button>` `.click()`
reaches directly — no Floating-UI portal timing to fight, unlike `popover`'s
trigger.

## 0. Does `command` compose the already-ported `dialog.rs`? **No — it is distinct, the same way `AppDialog` is**

`native/mapping/dialog.md` models `DialogPopup`
(`crowbar_ui::components::dialog::Dialog`) plus whichever of
`DialogHeader`/`DialogTitle`/`DialogDescription`/`DialogFooter` a call site
nests, and explicitly **excludes** `AppDialog` — `settings-dialog`,
`unsaved-changes-dialog`, `file-explorer-dialogs`, etc. — because that file
bypasses all four and hand-rolls its own chrome from raw `DialogPortal`/
`DialogBackdrop`/`DialogViewport`/`DialogPrimitive.Popup`.

`command.tsx`'s `CommandDialogPopup` does **exactly** what `AppDialog` does:

* it imports `@base-ui/react/dialog` **directly** as `CommandDialogPrimitive`
  — not `dialog.tsx`'s own `Dialog`/`DialogPopup` exports;
* it composes `CommandDialogPortal`+`CommandDialogBackdrop`+
  `CommandDialogViewport`+`CommandDialogPrimitive.Popup` itself;
* `CommandInput`/`CommandPanel`/`CommandFooter` stand in for
  `DialogHeader`/`DialogTitle`/`DialogFooter` — there is no
  `DialogHeader`/`DialogTitle` anywhere in the file.

So **`command` is distinct from `dialog.rs`, not a caller of it** — the same
underlying base-ui `Dialog` primitive, a different composition, exactly
`AppDialog`'s relationship to it one file over (and `AppDialog` itself remains
its own, unported §6.2 item, exactly as `dialog.md` §5 leaves it).

What **is** shared with `dialog.rs` is the **wrap technique**, restated in
full because it is the same vendor widget: `GpuiDialog` is neutralised in
place (`.overlay(false).close_button(false)`, every default field zeroed,
`.min_h(px(0.0))` against the vendor's own 96px floor — `dialog.rs`'s P3.28
finding, applied here even though this surface's own body never falls under
it either) and this crate's own `div()` carries the real chrome one level in.
The one number that differs: `command.tsx`'s `max-w-xl` (**576px**) where
`dialog.tsx`'s two reachable call sites cap at `max-w-md` (448px) — a
call-site number on both files, not a primitive one.

`SurfaceParams::render_ctx` (P3.21's seam, `dialog.rs`'s own finding) is what
lets `Command::render` take `window`/`cx` the way `Dialog::render` does —
`GpuiDialog::new` mints a `FocusHandle` off `cx` regardless of which
composition sits around it.

## 1. Values — the popup

`CommandDialogPrimitive.Popup`. Measured with `getComputedStyle` on the live
element, pinned at rest (§6).

| React / Tailwind | Compiles to (measured) | gpui / `crowbar-ui` | Oracle |
|---|---|---|---|
| `w-full`, capped by `max-w-xl` | `width: 576px` at a 1714px viewport | `Command::max_width` + `window.viewport_size()`, by hand | `bounds.w` = 576 |
| `rounded-2xl` | `border-radius: 18px` | `theme.radius_2xl` | `radius` = 18 |
| `border` | `border-width: 1px`, `oklch(1 0 0 / 0.06)` | `.border(BORDER_WIDTH).border_color(theme.border)` on this crate's own inner div | `border.w` = 1, compared exactly |
| `bg-popover` | `oklch(0.239 0.002 106.5)` | `theme.popover` | `bg` |
| `text-popover-foreground` | `oklch(0.97 0 0)` | `theme.popover_foreground` | `fg`, inherited |
| `max-h-105` | `420px` — **not exercised**, the reachable body (140px) never approaches it | `MAX_HEIGHT`, carried not enforced | no cell reaches it |
| `shadow-lg/5`, `before:shadow-[…]`, `transition-*`/`data-*-style` | — | nothing | `ANCHORS.md` §6, `dialog.md`'s own reasoning applies unchanged |

## 2. Values — the wrapper, the input row, the panel and the footer

| React / Tailwind | Compiles to (measured) | gpui / `crowbar-ui` | Oracle |
|---|---|---|---|
| `Command` (`data-slot="command"`), the caller's own `className` | `574×140` — no default styling of its own | `Command::render`'s inner `div`, unanchored (see §3) | not anchored |
| `CommandInput`'s unnamed `.px-2.5.py-1.5` wrapper | `574×48`, no `data-slot` in the source at all | `INPUT_ROW_PADDING_X`/`_Y`, unanchored | not anchored — see §3 |
| `CommandPanel`, **no `data-slot` in the source** | `576×47` (`-mx-px`, 2px wider than the wrapper) | `ID_PANEL = "command-panel"` — this port's own addition | `bounds`, `bg`, `border`, `radius` |
| `CommandPanel` `border border-b-0` | `border-width: 1px` top/left/right, `0` bottom | `.border(BORDER_WIDTH).border_b(px(0.0))` | `border.w` = 1 |
| `CommandPanel` `rounded-t-xl` | `14px` (top corners; bottom is `0`, a footer is present) | `theme.radius_xl` | `radius` |
| `CommandFooter` `border-t` | `border-width: 1px` | `.border_t(BORDER_WIDTH)` | `border.w` = 1 |
| `CommandFooter` `px-5`/`py-3` | `20px`/`12px` | `FOOTER_PADDING_X`/`_Y` | `bounds` |
| `CommandFooter` `rounded-b-[calc(var(--radius-2xl)-1px)]` | `17px` | `theme.radius_2xl.value() - BORDER_WIDTH` | not directly observed (this footer's own top-left is square) |
| `CommandFooter` background | **transparent** — no `bg-*` class in the source | nothing painted | `bg` shows the popup's own `bg-popover` through it |

### `command` and the input-row wrapper carry no anchor, on purpose

Neither has a `data-slot` in the source, and neither paints anything of its
own that this contract can see beyond position and extent — `command`'s
`className` is entirely the caller's, and the input row's box is fully
accounted for by `Command::input_row_height()`. `command-panel` is the
opposite case: real border/background/radius with **no** `data-slot` at all
— this port adds `data-oracle-id="command-panel"` to it, because a painted
box needs an anchor whether or not the React file happened to name it, the
same call `dialog.rs`'s own root anchor never had to make (its box already
had one).

### Declarations

* `CONTENT_SIZED = []`. The popup's width is a computed length; the panel and
  the footer are the popup's own width less border/margin; nothing here is a
  box whose used width is a text run's.
* `LINE_SIZED = []`. Unlike `dialog`'s title, this surface has no anchor that
  is a bare `leading-none` text run — `command-item`'s content (the nearest
  candidate) is call-site-owned and unanchored, see `autocomplete.md` §3.

## 3. The body is a height, folded from three pieces, not a single field

Unlike `dialog`'s `body_height` (one call-site-owned quantity), this
surface's body is **three** pieces, each itself either a reused
`autocomplete` struct or a measured constant: the input row
(`INPUT_ROW_PADDING_Y×2 + Input::control_height()`), the panel
(`BORDER_WIDTH + <empty state> + List::height`), and the optional footer.
`Command::popup_height()` folds them; `row_layout/command.rs`'s own tests
drive each term independently (footer height, footer presence, max-width)
the same way `dialog.rs`'s do.

## 4. What wrapping cost — the same three things `dialog` needed, restated

| | |
|---|---|
| `Command::render` needs `window`/`cx` | `GpuiDialog::new` mints a `FocusHandle` off `cx`; `render_ctx` (P3.21's seam) carries it through `render_row`, unchanged from `dialog`'s own account. |
| `.overlay(false)` / `.close_button(false)` | Identical reasoning to `dialog.rs` — no `Root` mounted here, and the vendor's close affordance would trip its own gap logic with a second child. |
| The outer `GpuiDialog` neutralised in place | `.p_0().bg(TRANSPARENT).border_0().border_color(TRANSPARENT).rounded(px(0.0))` — same field-by-field reasoning, restated because it is the same vendor default set regardless of which composition sits around it. |
| `GpuiDialog::w`/`max_w` are fixed-pixel fields | `w-full max-w-xl` reproduced by hand against `window.viewport_size()`, `command.rs`'s own `VIEWPORT_PADDING` (16px — the same number `dialog::VIEWPORT_PADDING` carries, independently authored, not shared). |
| `.min_h(px(0.0))` | The vendor's own unconditional 96px floor (P3.28's finding) — this surface's own body (140px) never falls under it either, applied anyway because the floor is the vendor's property, not this call site's. |

**Verdict: strict parity is reached on every field the contract carries** —
the popup, the panel, the footer, and (via `autocomplete.rs`) the input group,
the field, the empty state, the item and the list. No property resisted
styling; nothing here needed a fourth layer beyond `dialog`'s own three.

## 5. Reachability

`workspace-switcher.tsx`'s `WorkspaceSwitcherMenu`, opened from
`context-pill.tsx`'s "Switch workspace" button — a plain `<button>`
`.click()` reaches directly. `code-block-node.tsx` names this file in a
comment (*"`@/components/ui/command` wraps this app's own base-ui
`Autocomplete`… This file talks to `cmdk` and `@radix-ui/react-popover`
directly instead, mirroring the shape of the (skipped) registry
`command.tsx`/`popover.tsx`"*) but does not import it — confirmed by grep, it
imports `cmdk`'s `Command` (a different package) and is already recorded as
Plate-only, out of Phase 3 scope. So `command.tsx` has exactly the two
consumers named above, and both compose to one reachable palette.

## 6. ‼️ The dev server serves the shared worktree, so this branch's `data-oracle-id`s are not live — `dialog.md` §6's wall, met again

Following that item's own workaround exactly: `getComputedStyle`/
`getBoundingClientRect` on the live, unmodified
`[data-slot="command-*"]`/`[data-slot="autocomplete-*"]` elements reproduce
every field `extractSnapshot` would compute — the two functions read the same
DOM properties. The mount transition was pinned at rest by the same
manoeuvre (`command-dialog-popup.style.transition = 'none'`; confirmed
`transform: none`, `opacity: 1`, no `data-starting-style` before any number
was read). `/tmp/p3-ref-command.json` is hand-assembled from these live
readings rather than machine-emitted by `extractSnapshotSource` end to end.

**Per-element identity**, as the brief requires — none of these was
constructed, injected or stubbed; every one is a real primitive read off the
live document:

| element | `className` (non-empty) | `data-slot` | `innerText` | real primitives under `command-dialog-popup` |
|---|---|---|---|---|
| popup | yes (`relative row-start-2 flex …`) | `command-dialog-popup` | — | 1 |
| panel | yes (`relative -mx-px …`) | *(none — this port's addition)* | — | 1 |
| footer | yes (`flex items-center justify-between …`) | `command-footer` | `"NavigateOpenEsc Close"` (concatenated text nodes) | 1 |
| item | yes (`min-h-8 cursor-default …`) | `command-item` | `"oracle-fixture / home"` | 1 (one workspace in this dev environment) |

`data-slot="command-item"`/`"command-list"`/`"command-empty"` mask
`autocomplete.tsx`'s own `data-slot`s on the identical DOM nodes (their own
`{...props}` spread wins) — the `data-oracle-id`s this port added stay
`autocomplete.tsx`'s own regardless, per `autocomplete.md` §0's finding, so
the two vocabularies (`data-slot` vs `data-oracle-id`) genuinely disagree on
this one composed cell and that disagreement is recorded rather than
silently resolved either way.

The footer also carries four `<Kbd>` instances — `kbd.tsx`'s own primitive,
already ported, each `data-oracle-id="kbd"` — which is why `command`'s
`oracleSurfaceScope` entry (`web/src/lib/oracle/extract.ts`) exists at all:
an undeclared capture rooted at `command-dialog-popup` pulls all four in, a
repeated-id document `ANCHORS.md` v1.8 refuses outright. See that entry's own
comment for the full account.

## 7. The two-frame delivery, inherited rather than rebuilt

Exactly `dialog.rs`'s own §7: `gpui_component::Dialog`'s render path draws
its full content synchronously (no `.trigger()` set, so it never gates on a
captured trigger bound), and the outer box's `anchored()` positioning is
`gpui`'s own deferred/anchored primitive — the shared `row_layout.rs` harness
delivers it correctly with nothing local to `row_layout/command.rs` needed
for it.

## 8. Two real defects a driver snapshot caught, and one cross-surface finding it surfaced

Every number in §1–§2 was cross-checked against the real thing three ways —
the live browser capture (§6), `row_layout/command.rs`'s `#[gpui::test]`
suite, and `CROWBAR_ROW_SNAPSHOT=<path> cargo run --bin crowbar-app --features
driver -- --surface command --width 1714 --viewport-width 1714`, which opens
a real window and runs the actual binary's render path. The third one caught
two bugs the first two did not:

* **`autocomplete-input`'s anchor was the control's box, not the field's.**
  A first draft anchored `control.child(field)` — the whole `554×36` control
  — under `ID_INPUT`, on the theory that "the field's box with the control's
  paint on it" (the module docs' own phrase) meant returning one combined
  `Div`. The driver snapshot reported `554×36` at the control's own origin
  instead of the live `552×34` inset by the border; the fix anchors the
  **field**, a normal-flow child of an unanchored control (whose `border(1px)`
  insets it automatically), and re-running the snapshot reproduced `552×34` at
  `(1,1)` inside the control exactly.
* **`AutocompleteEmpty` is a sibling of `AutocompleteList`, not nested inside
  it.** `autocomplete.rs`'s first draft folded `ListContent::Empty` into
  `List::render`, reasoning that both are usually reached through the same
  call site. `workspace-switcher.tsx`'s actual JSX is
  `<CommandPanel><CommandEmpty/><CommandList>…</CommandList></CommandPanel>` —
  siblings, both direct children of the panel. The driver snapshot placed
  `autocomplete-empty` at the *item's* padded position (`(9,58)`, one
  `list_padding` step inside the scroll area) instead of the live `(1,50)`
  (the panel's own content-box origin, one border-width in, before any list
  padding); the fix pulled `empty()` out into its own function that
  `command.rs`'s `panel()` calls directly, in DOM order, before `list.render`.
  Re-running the snapshot reproduced `(1,50)` exactly.

Both are recorded here rather than silently folded into a clean §1/§2,
because a wrap this port claims strict parity on should show its work: the
driver run that caught them is reproducible with the command above, and the
row-layout tests this item ships (`the_item_is_...`, the panel/footer width
tests) now pin both numbers so neither regresses unnoticed.

**Cross-surface finding, not a defect:** `input.tsx`'s own `<span
data-oracle-id="input-control">` is a **hardcoded literal**, never spread
from props — confirmed by reading the source — so `AutocompletePrimitive.Input`'s
merged props (this item's own `data-oracle-id="autocomplete-input"` included)
cannot reach it; they land on the *inner* `<input>` instead, where
`data-slot`'s own override already put `autocomplete-input`. The composed
`command` DOM therefore carries `input-control` (the already-shipped `input`
surface's own anchor) on the live control element, unconditionally, as a
structural side effect of `AutocompleteInput`'s `render={<Input/>}`
composition. `command`'s own `oracleSurfaceScope` entry does not declare it
(it belongs to a different surface — the same reasoning that keeps
`autocomplete-item` and the footer's `kbd`s out of the declared set), so an
automated capture drops it; a future reader who sees `input-control` in an
*undeclared* capture of this surface should not read it as a bug.

## 9. Real-window verification is currently environment-limited — reported, not silently skipped

The driver command in §8 opened a genuine window and, on its first two runs
(right after the panel-width fix, before the empty-sibling fix), reproduced
`command-dialog-popup` at the requested `576×142` exactly. On later runs —
after the screen locked and unlocked once mid-session (confirmed:
`system_profiler` hung for several minutes, then returned) — the same
command reproducibly reported the popup at `537×142` instead, on a machine
whose actual display is `3024×1964` (logical ≈`1512×982`), narrower than the
requested `1714px` viewport. No stray `crowbar-app` process was found
running underneath it. This reads as the OS clamping a real window that
cannot fit the current screen, a condition the row-layout harness (a `gpui`
`TestAppContext` window, sized to the request regardless of the physical
display) does not share — and did not regress: the full row-layout suite,
run immediately after, still passed every assertion at the exact live
numbers. The two real-window runs that did match are the ones this item's
own numbers are drawn from; later real-window widths are not, and are named
here rather than silently reconciled.
