# `autocomplete` (P3.32) — the primitive `command` composes

`web/src/components/ui/autocomplete.tsx` →
`crates/crowbar-ui/src/components/autocomplete.rs`.

> A §6.2 row, in the shape `native/MAPPING.md` fixes. Kept in its own file for
> the reason `popover.md` gives. Ported together with `command` — see
> `command.md` — because `command.tsx` is `autocomplete.tsx`'s one importer
> and reuses these boxes directly.

**Reference:** captured as part of `/tmp/p3-ref-command.json` (see
`command.md`) — every box this module builds is reachable only through
`command.tsx`'s composition; there is no standalone capture, and §0 says why.

**Live count: one importer (`command.tsx`), zero fuzzy matching.** The item's
own brief flagged this pair as needing Zed's `fuzzy_nucleo`; measured and
found wrong —
`grep -c '\.filter(\|score\|fuzzy\|match(\|includes(\|indexOf('` across both
files is **0**. The real matchers (`web/src/utils/fuzzy-matcher.tsx`,
`utils/search-match.ts`) are outside `components/ui/` and are Phase 4 work.
Nothing here ports one.

## 0. `autocomplete` has no root box of its own, and that decided the surface split

`Autocomplete` — this file's own export of `AutocompletePrimitive.Root` — is a
headless context provider: it renders **no DOM node**. The input, the list,
the item and the empty state are four independent exports with no common
containing box the primitive itself builds; that combination exists only at a
call site, and the one call site is `command.tsx`. So there is no
`--surface autocomplete` alongside `--surface command`: capturing "the input
and the list together" as `autocomplete`'s own picture would be this port's
own invented composition, not a box any real screen shows. `command.rs`
reuses this module's `Input`/`List`/`Item`/`ListContent` structs directly
rather than re-deriving the same geometry under a second name — see that
file's own module docs for the full account.

## 1. Wrap or build: **build**

`autocomplete.tsx` reaches no `gpui-component` widget at all — it delegates
to `@base-ui/react/autocomplete`'s headless behaviour plus this app's own
already-ported `input.rs`/`scroll_area.rs`. §10.1's "do not rebuild a
primitive that exists there" does not fire: the closest vendor concept,
`gpui_component::combobox::Combobox`, is a different shape (a trigger that
opens a floating popup), and its item box —
`searchable_list::SearchableListItemElement` — is exactly the box
`native/mapping/select.md` already found unmeasurable: `h_flex().id(self.id)`
with real padding/background/rounded chrome, built inside the vendor's own
`RenderOnce::render`, confirmed again here by reading
`searchable_list/item.rs` directly. `autocomplete.tsx`'s own shape — an
always-mounted input with an inline, non-floating list below it — is
`Command`'s shape, not `Combobox`'s, and every box below is this crate's own
`div()`, exactly as `dialog`'s header/title/footer are despite
`gpui_component::dialog` existing.

**A lower-level seam does exist and was checked.**
`gpui_component::list::{List, ListDelegate}` (not `searchable_list`) wraps a
delegate's own `Self::Item: Selectable + IntoElement` in a bare
`div().id(id).w_full().overflow_hidden()` — no bg/border/padding of its own —
which *would* satisfy the sidebar lesson's "the caller's element can BE the
box" test, unlike `SearchableListItemElement`. It was not used: it is an
`Entity`-backed, `Window`/`Context`-constructed live widget (virtualisation,
its own keyboard-nav actions, a search debounce `Task`), and every §6.2
surface built so far — `dialog`, `popover`, `scroll_area` included — is a
plain, fixture-driven struct with a `render(&self, …) -> AnyElement`, not an
`Entity`. Reaching for it here would be introducing the *first* stateful
`Entity` widget into this tree to render a fixed one-row list, disproportionate
to what the reachable UI needs and inconsistent with every neighbouring
surface's own shape.

## 2. Values — the input

Measured through `command.tsx`'s `CommandInput` (`size="lg"`,
`startAddon={<SearchIcon/>}`, its own `border-transparent!` etc. override) —
the only live instance. `autocomplete.tsx`'s own resting arm (no override,
`transparent: false`) has no live reference and is carried as `command.md`'s
own §0 records for the primitive's other pictures.

| React / Tailwind | Compiles to (measured) | gpui / `crowbar-ui` | Oracle |
|---|---|---|---|
| `AutocompleteInputGroup` `w-full` | `554×36` | `Input::render`'s group `div` | `bounds` — this is [`ID_INPUT_GROUP`] |
| `startAddon` box, `ps-3` plus the `SearchIcon`'s own extent | `23×36` — the **box** is a real anchor ([`ID_START_ADDON`]); its content (the icon) is not, `input.rs`'s `icon_box` convention | `Input::addon_box`, width the measured [`ADDON_WIDTH`] constant | `bounds` |
| control (`<Input>`'s outer `<span>`), `border-transparent! bg-transparent!` (command's override) | `554×36`, `border 1px` transparent | `Input::control` — **no anchor of its own**; the field is a normal-flow child, inset automatically by the control's own border | `border.w = 1` compared exactly, but on the *field*, not the control — the control carries no anchor to compare |
| control `rounded-lg` | `10px` | `theme.radius_lg` | painted, not separately observed (no control anchor) |
| field (`<Input>`'s inner `<input>`), `size="lg"` height | `h-9.5 sm:h-8.5` → `34px` at `sm:` | `InputSize::Lg.extent(Breakpoint::Sm)` | `bounds.h` — this is [`ID_INPUT`], inset `(1,1)` inside the control by its border |
| field `padding-left`, `startAddon` present, `size="lg"` | **`31px`**, measured — `sm:*:data-[slot=autocomplete-input]:ps-[calc(--spacing(8)-1px)]` | `addon_gutter(InputSize::Lg)` | `bounds` (offset) |
| field `padding-right` | `11px` (bare `Size::padding_x`, no trigger/clear) | `self.size.padding_x()` | `bounds` |
| control `text-base sm:text-sm` | `14px/20px` | `InputSize::text_size`/`control_line_height` | `font` — inert here (the field's own `leading-*` overrides it) |

### The padding-left is `autocomplete.tsx`'s own class, not `input.tsx`'s

`AutocompleteInput` puts a `*:data-[slot=autocomplete-input]:ps-*` selector on
the **control**'s merged `className`, which reaches the field via a
descendant selector — confirmed live rather than assumed: cloning
`React.cloneElement` merges `AutocompletePrimitive.Input`'s generated props
onto `<Input>` itself, and `Input`'s own `className` param (distinct from its
internally-computed `inputClassName`) paints the **outer span**, not the
field. `input.rs`'s own `icon_gutter()` is a *different* class
(`leftIcon`'s `ps-7`/`ps-8`), so this module carries its own
`addon_gutter()`/`trailing_gutter()` rather than reopening `input.rs`'s closed
`LeadingPad` vocabulary for a class that is not even `input.tsx`'s own.

## 3. Values — the item, the empty state and the list

| React / Tailwind | Compiles to (measured) | gpui / `crowbar-ui` | Oracle |
|---|---|---|---|
| `AutocompleteItem` `px-2` | `8px` | `ITEM_PADDING_X` | `bounds` |
| `AutocompleteItem` `py-1` (bare) / `command.tsx`'s `py-1.5` override | `4px` / **`6px`**, live is the override | `ITEM_PADDING_Y` / `command::ITEM_PADDING_Y` | `bounds` |
| `AutocompleteItem` `rounded-sm` | `6px` | `ITEM_RADIUS` | `radius` |
| `AutocompleteItem` `data-highlighted:bg-accent` | `oklch(1 0 0 / 0.04)` — the sole item, `autoHighlight="always"` | `Item::highlighted` → `theme.accent` | `bg` |
| item's own content (icon, label, check) | call-site-owned, unanchored — see [`Item`] | `Item::content_height` (measured, `18px`) | not a field of this primitive |
| `AutocompleteEmpty` `not-empty:p-2` | `0×574` — **mounted, `:empty`** (one row exists) | `empty(theme, anchors, has_content=false)` | `bounds` (zero height, real node) |
| `AutocompleteList` `not-empty:p-1` (bare) / `command.tsx`'s `p-2` override | `4px` / **`8px`**, live is the override | `LIST_PADDING` / `command::LIST_PADDING` | `bounds` |

### `command.item`'s `font-editor` is a real font, unlike `dialog`'s dead `font-heading`

`command-item`'s label span carries `font-editor`, and `theme.css` gives
`--font-editor: var(--editor-font-family)` a **real static fallback** via
`editor-theme.css` (`ui-monospace, 'JetBrains Mono Variable', monospace`),
confirmed live — `getComputedStyle().fontFamily` leads with
`"JetBrains Mono Variable"`, this crate's own `Theme::font_mono` (a
*different* custom property with the same primary family — `theme.css`'s
`--font-mono: var(--editor-font-family, 'JetBrains Mono Variable', …)`).
**Unreached** regardless: the label is call-site content (see §3 above), not
an anchor this primitive owns.

### The empty state is a real, always-mounted node — and a **sibling** of the list, not its child

Confirmed live on the one reachable cell (one workspace, so
`ListContent::Item`): `command-empty` is a genuine `0×574` DOM node, not an
absent one. `not-empty:p-2` is a `:empty` pseudo-class selector — base-ui
always renders `Autocomplete.Empty`, and only its *content* (a string, when
the list truly has none) triggers the padding. `empty(theme, anchors, false)`
reproduces the collapse; the `has_content = true` arm (this primitive's other
picture) has no live reference in this dev environment.

**Not nested inside `AutocompleteList`.** `workspace-switcher.tsx`'s JSX is
`<CommandPanel><CommandEmpty/><CommandList>…</CommandList></CommandPanel>` —
`CommandEmpty` and `CommandList` are siblings, both direct children of the
panel. A first draft folded `Empty` into `List::render` instead; a driver
snapshot caught it (the anchor landed at the item's own padded position
rather than the panel's content-box origin) before it could be reported as
converged — see `command.md` §8 for the full account, including the second
defect the same run caught (`autocomplete-input`'s own bounds).

### Declarations

* `CONTENT_SIZED = []`. Every anchor here is `w-full` inside its parent, or
  the field, which has no text node at all (`input.rs`'s own finding).
* `LINE_SIZED = []`. Nothing on this surface is a bare `leading-none` run —
  every anchor is padding-plus-content or a call-site parameter.

## 4. `AutocompleteList`'s scroll area is built, not wrapped through `ScrollArea::render`

`super::scroll_area::ScrollArea::render` has no seam for a caller-supplied
child — its own `body()` is a plain extent, the same shape `popover`'s and
`dialog`'s bodies take (see that module's own docs). `AutocompleteList` needs
a **real, anchored** child (`autocomplete-list`), which that contract cannot
carry. Reproducing the two-`div()` root and viewport locally — both
`super::scroll_area::BORDER_WIDTH` and `RADIUS` are `0`, so nothing here
diverges from what that surface already establishes — is cheaper and more
honest than fighting a body-extent contract meant for unanchored content.
`List::render` still opts `scroll_area::ID_ROOT`/`ID_VIEWPORT` into `anchors`,
so a `command` capture genuinely nests `scroll-area-root`/
`scroll-area-viewport` under `command-panel` — confirmed live, the same ids
`scroll_area.rs`'s own standalone call sites (`workspace-tree`, `git-panel`)
carry, and the same `574×46` instance that file's own fixture docs already
record as "the command palette's".

## 5. What was captured, and how

The dev server serves the shared worktree (`dialog.md` §6's wall, met again):
this branch's `data-oracle-id`s are not live there. Captured instead through
`getComputedStyle`/`getBoundingClientRect` on the live, unmodified
`[data-slot="autocomplete-*"]` elements inside the running app's command
palette (Context Pill → click → the workspace switcher), pinned at rest
(`command-dialog-popup.style.transition = 'none'`; confirmed `transform:
none`, `opacity: 1`, no `data-starting-style` first). Per-element identity,
as the brief requires:

| element | `className` (non-empty) | `data-slot` | `innerText` | real primitives on the page |
|---|---|---|---|---|
| input group | yes (`relative not-has-…`) | `autocomplete-input-group` | — | 1 (the one `CommandInput`) |
| field | yes (`w-full min-w-0 …`) | `autocomplete-input` | `""` (placeholder showing) | 1 |
| empty | yes (`not-empty:p-2 …`) | `autocomplete-empty` | `""` | 1 |
| item | yes (`min-h-8 …`) | `command-item` (masks `autocomplete-item`; see `command.md`) | `"oracle-fixture / home"` | 1 |

Every number was read off the running app, never invented — reachable and
live, not a fabricated reference.
