# `number-input` (P3.27) — a row-of-three built from a private local copy of the shared `Button` shell

`web/src/components/ui/number-input.tsx` (`export default NumberInput`) →
`crates/crowbar-ui/src/components/number_input.rs`,
`crates/crowbar-app/src/{surfaces,row_layout}/number_input.rs`.

> A §6.2 row, in the shape `native/MAPPING.md` fixes. Kept in its own file
> because P3 runs several workers in parallel and one appended table is one
> conflict per item.

A `flex items-center gap-1` row: two `<Button variant="ghost" compact>` icon
buttons flanking a plain `<input type="text">`. Every "Compiles to" below was
measured with `getComputedStyle`/`getBoundingClientRect` on the live app —
pid connected via the Tauri MCP bridge, `innerWidth` 1714, `html.dark`, the
Settings dialog's Appearance tab.

**Reference:** `/tmp/p3-ref-number-input.json`, captured live through
`extractSnapshotSource` (`web/src/lib/oracle/extract.ts`) from Appearance →
"UI Font Size" — `size="xs"`, `.number` width (`w-28`) — at a 1714px
viewport, dark, resting (`value="15"`).

**Live count: 15 call sites, all reachable, and confirmed by import.** Every
`NumberInput` importer resolves to a settings tab reached through the app's
own Settings dialog: `file-tree-settings.tsx` ×1 (Indent Size),
`appearance-settings.tsx` ×2 (UI Font Size, Keep Workspaces in Memory),
`developer-settings.tsx` ×2, `terminal-settings.tsx` ×5, `editor-settings.tsx`
×5. Every one of the 15 passes `size="xs"` — the only size any live call site
requests — and merges one of exactly three widths from
`web/src/features/settings/components/settings-control-widths.ts`:
`SETTINGS_CONTROL_WIDTHS.numberCompact` (`w-24`, 96px, 6 sites),
`.number` (`w-28`, 112px, 7 sites, including the captured reference) or
`.default` (`w-36`, 144px, 1 site — `terminalScrollback`).

## 0. Wrap or build: the seam test, applied

`native/vendor/gpui-component/src/input/number_input.rs` has a `NumberInput`,
and `native/vendor/gpui-component/src/stepper/stepper.rs` has a `Stepper` —
the name a bare member-name grep would flag for this item, and a false lead
this report exists to name explicitly: `stepper::Stepper` is a **multi-step
wizard progress indicator** (`StepperItem`s laid out in a row or column, each
showing a step number and a connecting line), not a numeric input's
increment/decrement control. It shares no shape, no anchor set and no
purpose with `number-input.tsx`, and the earlier item this brief warns about
("my seam survey over-counted 7 → 3") is exactly the failure mode a
name-only grep produces. `stepper.rs` is not examined further.

`gpui_component::input::number_input::NumberInput` is the real candidate, and
it does expose element-accepting seams — `.prefix(impl IntoElement)` and
`.suffix(impl IntoElement)`, which the popover-derived test this brief states
says to look for. But `RenderOnce::render` (`number_input.rs:283`) builds the
**whole visible tree** itself:

```rust
h_flex()
    .child(Button::new("minus")…)
    .child(Input::new(&self.state)….when_some(self.prefix, |this, prefix| this.prefix(prefix))…)
    .child(Button::new("plus")…)
```

`prefix`/`suffix` land **inside** the vendor's own private `Input`, as an
optional decoration on the field — never as the three top-level children
(`h_flex`'s own minus button, the field, the plus button) this surface needs
to anchor pixel-identical. And the real React component does not even reach
for the seam that exists: it flanks a plain `<input>` with two of **this
app's own** `@/components/ui/button` `Button`s, not `gpui-component`'s, and
its minus/plus icons are sized by `number-input.tsx`'s own `iconSizes` table
(`size-3`/`size-3.5`/`size-4`, flat — no `sm:` step), never by
`NumberInput`'s own size variant. So even where a seam exists, it does not
reach the three boxes this surface needs. **Verdict: built**, from raw
`div()`s — the same call `input`, `button`, `checkbox` and `radio_group` each
made, for the same reason `popover`'s own module docs give the general test:
*a widget is wrappable exactly when it lets the caller supply the element
that becomes the anchored box, not merely a style refinement on a box the
vendor already decided the shape of.*

### The stepper buttons are a private local copy, not a reuse of `button.rs`

The two flanking buttons *are* this app's shared `Button` component in
React, so reusing `crowbar_ui::components::button::Button` looked tempting
and was rejected for one measured reason: `button::Size::icon`/`glyph_box`
size a button's glyph off **`Button`'s own** size table
(`[&_svg:not([class*='size-'])]:size-4.5 sm:size-4` for the `default` text
size this call renders at, since no `size` prop is passed to `<Button>`).
`number-input.tsx`'s `Minus`/`Plus` icons carry their **own** explicit
`size-*` class, which beats that descendant selector outright and overrides
the glyph to `number-input.tsx`'s own `iconSizes` table instead — a
call-site override `button.rs`'s public API has no parameter for. Building a
small local element at this surface's own measured numbers is the same
choice `tooltip.rs` made for its shortcut chip (`keybinding.tsx`, ported as
a private helper rather than a fourth shared primitive), and the reason
every module doc in `crowbar-ui/src/components/mod.rs` gives for why these
surfaces stay independent: "components are ported independently and a
shared helper would make one surface's diff reach into another's file."

## 1. Values — the root

| React / Tailwind | Compiles to (measured) | gpui / `crowbar-ui` | Oracle |
|---|---|---|---|
| `flex items-center gap-1` | `display:flex`, `gap: 4px` | `.flex().items_center().gap(ROOT_GAP)` | `bounds` of children |
| call site's `w-24`/`w-28`/`w-36`, `max-w-full` | **96 / 112 / 144px**, authored | `Width` | `bounds.w` = 112 on the reference |
| *(none)* | `background: rgba(0,0,0,0)`, no border, no radius | no `.bg`/`.border`/`.rounded` | `bg` `#00000000`, `border.w` 0, `radius` 0 |

## 2. Values — the stepper buttons

Both flanking buttons are
`<Button type="button" variant="ghost" compact onClick=… disabled=…
aria-label=… className="shrink-0"><Minus|Plus className={iconSizes[size]} /></Button>`.
**`compact` is dead** — `button.tsx`'s own `ButtonProps` destructures it as
`compact: _compact` and never reads it — so this renders a plain
`variant="ghost"` button at the prop's own default size, `'default'`
(`h-9 px-[calc(--spacing(3)-1px)] sm:h-8`), **not** one of `Button`'s five
`icon*` square sizes. Confirmed by reading the live element's `className`
off the Settings dialog, not assumed from the prop name.

| React / Tailwind | Compiles to (measured) | gpui / `crowbar-ui` | Oracle |
|---|---|---|---|
| `h-9 sm:h-8` | **36 / 32px** | `button_height` | `bounds.h` |
| `px-[calc(--spacing(3)-1px)]` | **11px**, both breakpoints | `BUTTON_PADDING_X` | `bounds.w` |
| `border` | `border-width: 1px`, unconditional | `BUTTON_BORDER_WIDTH` | `border.w` = 1, exact |
| `border-transparent` (ghost) | transparent | `Color::TRANSPARENT` | `border.color` — v1.3: ignored while `w == 0`… but here `w` **is** 1, so the transparent colour genuinely is compared and genuinely is `#00000000` |
| `rounded-lg` | **10px** | `theme.radius_lg` | `radius` = 10 |
| *(no `bg-*` in ghost's resting rule)* | `background: rgba(0,0,0,0)` | no `.bg(…)` in the resting branch | `bg` `#00000000` |
| `hover:bg-accent data-pressed:bg-accent` | `background: var(--accent)` | `Buttons::hovered → theme.accent` | `bg`, see §5 |
| `Minus`/`Plus` `className={iconSizes.xs}` = `size-3` | **12×12px**, flat — no `sm:` step | `Size::icon_size` | invisible (unanchored glyph) |
| `[&_svg]:-mx-0.5` (the base button class list) | `-2px` margin each side on the glyph | `BUTTON_ICON_MARGIN_X` | folds into the button's own `bounds.w` |

**The button's width is genuinely content-sized in the DOM sense** — the
`default` size authors no `w-*`, only padding — **but not in `ANCHORS.md`
v1.5's sense**, which exists only to correct GPUI's `ceil()` on a *text
run's* max-content width. The glyph here is a fixed `size-3` box, not text,
so both engines compute `2×padding + glyph_box + 2×border` by ordinary flex
arithmetic with no rounding step for either side to disagree on — measured
at exactly **32px** on the live reference (`(11 + (12 − 2×2) + 11) + 2×1 =
32`), and `button_width` reproduces the arithmetic rather than a bare
literal. No `data-oracle-content-sized` is warranted on either button, and
this is the reasoned "why not" rather than an oversight.

## 3. Values — the field

An `<input type="text">`, so `input.md` §1 applies verbatim: it is a **void
element** with no text node, so the reference records only its box.

| React / Tailwind | Compiles to (measured) | gpui / `crowbar-ui` | Oracle |
|---|---|---|---|
| `h-6` (`xs`) | **24px**, flat — `fieldHeights` has **no** `sm:` step, unlike the buttons | `Size::field_height` | `bounds.h` |
| `rounded-md` | **8px** — not the buttons' `rounded-lg` | `theme.radius_md` | `radius` = 8 |
| `border border-border` | `1px`, `oklch(1 0 0 / 0.06)` | `.border(FIELD_BORDER_WIDTH).border_color(theme.border)` | `border.w` = 1, `border.color` compared |
| `bg-muted` | `oklch(1 0 0 / 0.04)` — the **bare** token, no `.mix()` | `theme.muted` | `bg` = `#ffffff0a` |
| `px-2` (`xs`/`sm`), `px-3` (`md`) | **8 / 12px** | `Size::field_padding_x` | `bounds` via content box |
| `ui-text-sm` (`xs`/`sm`), `ui-text-base` (`md`) | **12px/18px**, `14px/20px` | `theme.ui_text_sm`/`ui_text_base` | invisible — no anchor paints text |
| `text-foreground` | `oklch(0.97 0 0)` | `theme.foreground` | invisible |
| `min-w-[5ch]` | **37.26px** (`xs`/`sm`, `ui-text-sm`), **43.47px** (`md`, `ui-text-base`) — a font metric, measured via `getComputedStyle().minWidth`, not `5 ×` anything | `Size::min_field_width` | folds into `bounds.w`, see §4 |
| `flex-1` | flex-basis 0, grows/shrinks to fill the row | `.flex_1()`, clamped at `min_field_width` | `bounds.w` |
| `tabular-nums` | uniform digit widths — part of the **field's own** base class list, not a call-site option | baked into every fixture | affects `min-w-[5ch]`'s own px value, not separately visible |

`disabled:opacity-50` and `placeholder:text-muted-foreground` are in the
class list and **practically unreachable**: `NumberInput.formatValue` never
returns an empty string (it falls back to `'0'`), so the placeholder never
paints on any live cell, and no call site ever passes `disabled`.
`ANCHORS.md` §6 has no field for either rule regardless.

## 4. The field's `min-w-[5ch]` overflows the row at the narrowest authored width — a real trap

Measured on the live `Indent Size` cell (`.numberCompact`, `w-24` = 96px),
raw document coordinates: `dec.right=1333`, `field.left=1337`,
`field.right=1374.25`, `inc.left=1378.25`, `inc.right=1410.25`,
`root.right=1397` (`root.left=1301`):

```text
root   0,0     96×32   (authored, w-24)
dec    0,0     32×32
field  36,4    37.25×24   ← overflows: 36 + 37.25 = 73.25, inc starts at 40
inc    40,0    32×32      ← right edge 72, 13.25px PAST the root's own 96
```

The flex division gives the field `96 − 2×32 − 2×4 = 24px`, but its floor is
`min-w-[5ch] = 37.26px` — **wider than the space flex would give it** — so
the field, and the increment button after it, spill **13.25px past the
root's own right edge**. `flex-1`'s `flex-shrink: 1` cannot shrink the field
below its own `min-width`, and nothing on the row clips the overflow.
`NumberInput::field_width` reproduces exactly this: `max(flex_share,
min_field_width)` — confirmed both in a unit test against the arithmetic and
in a real `row_layout` window, where taffy independently overflows the same
13.25px. `.number` (the reference's own cell, 112px) and `.default` (144px)
both clear the floor and stay inside the row.

## 5. The state axis: `hover` is real, and it is the only one

`number-input.tsx` itself contains the substrings `hover`, `focus`,
`selected`/`data-selected` and `aria-invalid` **zero times each** — grepped,
not assumed, `checkbox.rs`'s own discipline. So `empty`, `selected`, `focus`
and `error` are all genuinely unmodelled: the original has no such rule to
disagree with, not merely one this port declines to reach.

`hover` is the one exception, and it comes from the **composed** `<Button
variant="ghost">` rather than from this file: `ghost`'s own `hover:bg-accent
data-pressed:bg-accent` is real, and `NumberInput::buttons.hovered` folds it
into the base style — never a `.hover(…)` refinement `ANCHORS.md` §6 says a
snapshot cannot see — the same way `button.rs` folds its own
`Interaction::hovered` in. **No reference** either way: synthetic pointer
events are denied on this project's machines, `button.rs`'s own standing
finding.

## 6. The `--viewport-width` axis moves only the buttons' height

Measured at both sides of 640px on the live `UI Font Size` cell (`.number`
width):

| | Base (< 640px) | Sm (≥ 640px, the reference) |
|---|---|---|
| button `h-9`/`sm:h-8` | **36px** | **32px** |
| button width | 32px (unchanged — padding and glyph carry no `sm:`) | 32px |
| field `h-6` (no `sm:` step) | **24px** (unchanged) | 24px |
| field `y` (centred by `items-center`) | `(36−24)/2 = 6` | `(32−24)/2 = 4` |

Only the buttons respond to the breakpoint; the field's own height is flat
across it, and the row's own height follows the taller button.
`NumberInput::render` reproduces this by giving only `button_height` a
`Breakpoint` parameter — `Size::field_height` takes none.

## `CONTENT_SIZED` / `LINE_SIZED`

Both empty. No anchor here paints a text run on either side of the contract:
the field is a void `<input>` (§3), and the buttons paint an icon, not text
— see §2's content-sizing note for why that specifically does not reach
v1.5.

## Capture evidence

Captured live through the Tauri MCP bridge against the running dev-desktop
instance (pid 61554, bundle `software.rabbyte.crowbar`), which serves the
**shared** `rewrite/rust/worktree`. This branch's `data-oracle-*` edits to
`number-input.tsx` were therefore applied with `setAttribute` to the live
"UI Font Size" `<NumberInput>` element immediately before extraction and
removed immediately after — the four attributes (`number-input`,
`number-input-decrement`, `number-input-field`, `number-input-increment`)
verified present on the correct four elements by reading them back before
capture. `extractSnapshotSource`'s generated IIFE was run through
`webview_execute_js` and its return value — the snapshot JSON — was the
tool's own synchronous result, so nothing round-tripped through a file sink.

Captured element identity, read off the live DOM rather than assumed: root
`className="flex items-center gap-1 w-28 max-w-full tabular-nums"` (3
children — the two buttons and the field); the field's own `value` was
`"15"`; both buttons' icon `<svg>` carried `className="size-3"`; the field's
`getComputedStyle` reported `fontSize: 12px`, `lineHeight: 18px` (`ui-text-sm`),
`paddingLeft: 8px`. Four real primitives on the page, not a stub — verified
by counting `document.querySelectorAll('button')` under the row (2) plus the
one `<input>`.

`window.innerWidth` was 1714 at capture time; theme was forced to `dark` via
`document.documentElement.classList.add('dark')` after the app's own Theme
Mode reverted to "system"/light across an unrelated reload — a transient,
non-persisted DOM class, not a stored preference change. The window was
resized to 1714×1119 via `manage_window` immediately before capture (an
earlier unrelated action had left it at 2400px logical width).

Independently re-confirmed after the port was written: `cargo run --features
driver -p crowbar-app -- --surface number-input --width 112
--viewport-width 1714 --class-width number` (`CROWBAR_ROW_SNAPSHOT` set)
emitted a snapshot whose four anchors match `/tmp/p3-ref-number-input.json`
field-for-field, including the two hex colours (`#ffffff0a`, `#ffffff0f`)
and the transparent button border (`#00000000`) — the binary's own output,
not the row-layout harness, read directly off disk.
