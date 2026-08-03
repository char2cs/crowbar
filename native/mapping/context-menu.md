# `context-menu` (P3.38) — covered by the native-menu ruling; no new surface built

`web/src/components/ui/context-menu.tsx` (396 lines) → **no new file in
`crowbar-ui` or `crowbar-app`, and no change to `crowbar-platform`.** The
scoping question the brief asks — is this a distinct anchor-diffed port, or is
it already served by the native-menu decision? — is answered **covered**, on
the same evidence shape `tree-row` and `primitive-dialog-service` reached
today. Nothing is built.

---

## 0. The file is two components, and only one of them is alive

`context-menu.tsx` exports two independent things that happen to share a file:

| Half | Built on | Exported as | Live importers |
|---|---|---|---|
| `ImperativeContextMenu` | `@base-ui/react/menu`'s **`Menu`** primitive (`MenuPrimitive.Root/Portal/Positioner/Popup/Item/Separator`) — the same import `dropdown-menu.tsx` uses | `ContextMenu` + `useContextMenu` | `tab-context-menu.tsx`, `use-file-explorer-context-menu.tsx` (both non-Plate; both import only `ContextMenu`/`ContextMenuItem`) |
| The declarative family (`ContextMenuRoot`, `ContextMenuTrigger`, `ContextMenuContent`, `ContextMenuCheckboxItem`, `ContextMenuRadioItem`, `ContextMenuLabel`, `ContextMenuSeparator`, `ContextMenuShortcut`, `ContextMenuGroup`, `ContextMenuPortal`, `ContextMenuSub`, `ContextMenuSubContent`, `ContextMenuSubTrigger`, `ContextMenuRadioGroup`) | `@base-ui/react/context-menu`'s **`ContextMenu`** primitive | 14 named exports | **zero**, checked one by one |

Checked with `grep -rl '\bContextMenu<Name>\b' web/src --include='*.tsx' --include='*.ts'`
against every one of the 14 declarative exports, excluding the definition file
itself: 12 return nothing at all; `ContextMenuTrigger` and `ContextMenuContent`
each return one hit, and both are a *comment* in `block-context-menu.tsx`
explaining why that file does **not** reuse these names (a `ContextMenu` name
collision with the imperative half — see that file's own header comment,
lines 6–10). `block-context-menu.tsx` instead imports
`@radix-ui/react-context-menu` directly and is Plate-only (`^block-`), already
out of scope by §3.2.

**So the declarative half is dead code today**, and porting it would be
porting something nothing renders — the same discipline `dropdown.rs`'s
`trigger()`/`item()` note and `select`/`sheet`'s "no surface" findings already
established for this codebase. It is not analysed further below.

The rest of this file is about the **live** half, `ImperativeContextMenu`.

## 1. `ImperativeContextMenu` is architecturally the same primitive `dropdown-menu.tsx` is, at the one call shape the native-menu ruling already covers

`grep -n "Menu as MenuPrimitive" web/src/components/ui/{context-menu,dropdown-menu}.tsx`
— both files import `Menu as MenuPrimitive` from `@base-ui/react/menu` and
render `MenuPrimitive.Root/Portal/Positioner/Popup/Item/Separator`.
`ImperativeContextMenu` is the *imperative* shape of that primitive (opened at
a `virtualAnchor` point rather than anchored to a trigger element), but it is
the same underlying `Menu`, not a different one.

`native/MAPPING.md`'s `# native-menu (P2.14)` section and
`native/oracle/blocked/s13-native-menus-accepted-delta.md` (read, not edited)
already settled this shape:

> "Dropdown menus should be native, not 'react' simulated." … a *context* menu
> is now an `NSMenu`

and `native_menu.rs`'s own module docs read `dropdown-menu.tsx`'s vocabulary
(`menu-item`, `menu-separator`, `menu-checkbox-item`, `data-disabled`,
`menu-sub-trigger`+`menu-sub-popup`) into `MenuItem`. `context-menu.tsx`'s
imperative half is not a second, unrelated component asking a fresh scoping
question — it is **the paradigmatic context-menu use** of the same base-ui
`Menu` primitive the ruling already named: two live call sites, both firing on
a right-click (`useContextMenu`'s `open` handler calls
`e.preventDefault()`/`stopPropagation()` on a `MouseEvent`, and
`use-file-explorer-context-menu.tsx`'s `handleContextMenu` is bound the same
way), both opened at a point rather than anchored to a rendered trigger. This
is exactly the shape §5.2's judged treatment already applies to
`dropdown-menu`'s own context-menu call site (`review-thread-item.tsx`'s
comment-actions menu).

**Answering the brief's Q1 directly: it is served by the existing decision,
and needs no wiring of its own** — not because a generic mapping argument
applies, but because reading the two live call sites (§2 below) shows they
need nothing beyond what `native_menu.rs`'s already-built vocabulary
(item/separator/submenu/checked/disabled) covers, and today **neither call
site has a Rust host to wire into at all** (§4).

## 2. What the two live call sites actually require — enumerated from the call sites, not from the type

The brief warns not to trust `context-menu.tsx`'s own `ContextMenuItem`
interface, since it already has one proven dead field (`keybinding`). Read
instead from `tab-context-menu.tsx` (18 `ContextMenuItem` object literals) and
`use-file-explorer-context-menu.tsx` (30), by grepping each for every key the
interface declares:

| `ContextMenuItem` field | Declared | Used by `tab-context-menu.tsx` | Used by `use-file-explorer-context-menu.tsx` | Rendered by `ImperativeContextMenu` at all |
|---|---|---|---|---|
| `id` | yes | every item | every item | yes — the React `key` |
| `label` | yes | every item | every item | yes — the row text |
| `icon` | yes | most items | most items | yes — `{item.icon}`, an arbitrary `ReactNode` (a `@phosphor-icons/react` glyph in both files) |
| `onClick` | yes | every item | every item | yes — the row's `onClick`, closes the menu unless `closeOnClick === false` |
| `separator` | yes | 3 uses (`sep-1/2/3`) | 5 uses (`sep-dir`, `sep-env-template`, `sep-file`, `sep-end`) | yes — renders `MenuPrimitive.Separator` instead of an item |
| `disabled` | yes | **0 uses** | **0 uses** | yes (passed to `MenuPrimitive.Item`), but no live cell exercises it |
| `shortcut` | yes | **0 uses** | **0 uses** | yes — a `<kbd>` — but nothing supplies one |
| `keybinding` | yes | **1 use** (`close`, line 194) | **0 uses** | **no.** Confirmed by reading the render loop (`context-menu.tsx:92-117`): only `item.shortcut` is read; `item.keybinding` has no consumer anywhere in the render tree. This is the same dead-wiring `native/QUEUE.md`'s `keybinding` "UNREACHABLE" note already recorded verbatim: *"`tab-context-menu.tsx` passes a `keybinding` prop that `context-menu.tsx` declares and never renders"* |
| `className` | yes | **0 uses** | **1 use** (`delete`, `'text-red-400'` — a destructive-red hint) | yes — merged via `cn()` |
| `items` (nested submenu) | yes | **0 uses** | **0 uses** | **no.** The render loop (`context-menu.tsx:92-117`) has no branch that reads `item.items` or recurses; a nested array on an object is silently inert |
| `closeOnClick` | yes | **0 uses** | **0 uses** | yes — but every real item defaults to closing, which is what both call sites want |
| checked/ticked | **not in the interface at all** | — | — | — |

**What the two call sites actually exercise, once the unused declared fields
are set aside: a flat list of `{id, label, icon, onClick}` rows plus
separators, with one destructive-styling hint (`className`) and one dead
`keybinding` prop.** No checkbox rows, no radio rows, no submenus, no disabled
rows, no shortcuts, no `closeOnClick: false`. That is a **strict subset** of
`crowbar_platform::native_menu::ContextMenu`'s vocabulary (item / separator /
submenu / checked / disabled), which is itself already built, tested (28
tests in `native_menu.rs`), and exercised end-to-end by the `native-menu`
driver surface against the equivalent live fixture.

Two things neither native menu implementation available in this repo can
carry, named rather than smoothed over: **icons** (dropped — the same "icons
are empty boxes" call `dropdown-menu`'s own `MAPPING.md` §2 makes for a GPUI
port, except here there is no box to paint at all: `AppKit`'s tick gutter is
the only per-row graphic `crowbar_platform::native_menu` draws) and the
**destructive-red `className` hint** on `use-file-explorer-context-menu.tsx`'s
`delete` row (a Tailwind color, not a menu concept `NSMenu` exposes without
`NSAttributedString` work neither native-menu implementation in this repo
does). Both are cosmetic losses of exactly the kind
`s13-native-menus-accepted-delta.md` already priced in for `dropdown-menu`:
*"the popup's background, radius, border, … are all AppKit's, not
theme.css's."*

## 3. Q2 — does this item constitute the "need" that keeps `crowbar-platform::native_menu` alive?

`native/oracle/blocked/s13-native-menus-accepted-delta.md` (read only, not
edited — the file is under `native/oracle/**`) already ruled on this, after
P2.14 shipped and then discovered the vendored menu:

> **"Call sites use the vendored menu."** It takes a gpui `Point<Pixels>`,
> dispatches `Action`s, and is what an idiomatic gpui call site wants; it also
> already covers Windows, which ours does not. … **`crowbar-platform::native_menu`
> is retained for now and retired before Phase 3 closes unless a concrete need
> appears that the vendored one cannot serve. It is wired to no call site —
> only to its own driver surface.**

What each actually provides, read from source rather than assumed:

| | `crowbar_platform::native_menu` (`crates/crowbar-platform/src/native_menu.rs`) | `gpui_component::native_menu::NativeMenu` (`native/vendor/gpui-component/src/native_menu/mod.rs`) |
|---|---|---|
| Vocabulary | item (id/title/enabled/checked), separator, submenu | item (label/disabled/checked/**icon**), separator, submenu |
| Dispatch | returns the chosen id **synchronously**, blocking `show_at` | dispatches a `Box<dyn Action>` via `Window::dispatch_action` — gpui's own idiom |
| Blocks the caller? | yes — `-popUpMenuPositioningItem:` runs its own tracking loop | **no** — "the OS tracking loop runs off GPUI's call stack" |
| Icons | **not modelled** | yes — file- or asset-backed, per-platform (`NSImage` template image on macOS, `HBITMAP` on Windows, `crate::Icon` fallback) |
| Platforms | macOS only (`#[cfg(target_os = "macos")]`); every other target gets `MenuError::Unsupported` | macOS, Windows, and a **drawn fallback** for anything else |
| gpui dependency | **none** — pure data + `objc2`, testable with no `App`/`Window` | requires `gpui::{App, Window}` throughout |
| From an existing `gpui::Menu` | no conversion | `impl From<gpui::Menu> for NativeMenu` — reuses an app's menu-bar definition directly |
| Wired to a call site today | **no** — only its own `--surface native-menu` driver | **no** — zero hits for `NativeMenu` or `gpui_component::native_menu` anywhere under `native/crates` |

The two live call sites' actual requirement (§2: a flat item/icon/onClick list
plus separators) is a subset of **both**, and the vendored one is a strict
superset of `crowbar_platform::native_menu` on every axis that matters to
those two call sites specifically — **icons**, which both call sites use on
nearly every row, and non-blocking dispatch through the GPUI idiom the rest of
`crowbar-app` is written in.

**This item does not constitute the concrete need `s13` names, and its
evidence points the other way.** If a context menu is ever wired to a Rust
tabs or file-explorer surface, the call site's own requirements — icons, a
non-blocking `Action`-dispatch call shape matching every other gpui-idiomatic
surface in this codebase — argue for `gpui_component::native_menu::NativeMenu`,
exactly as `s13`'s ruling already directs, not for extending
`crowbar_platform::native_menu`. Nothing here gives `crowbar-platform`'s copy
a reason to survive past the "retire before Phase 3 closes" date already
recorded; if anything, it removes one of the two candidate consumers that
might have supplied that reason. The two properties `s13` says would flip the
ruling — gpui-free testability, and a synchronous return value — are not
things either live call site asks for.

## 4. Why nothing is wired today, independent of §3

Even setting the retire-or-keep question aside: **neither live call site has a
Rust host to attach a menu to yet.** `crowbar-app`'s binary is, as of this
item, a matrix of isolated `--surface` drivers (`main.rs`'s own module docs:
*"one surface, drawn in one cell of the §8.3 matrix"*) — there is no running
tabs bar or file-explorer tree with a real right-click handler. Checked
directly:

```
$ grep -n "context.menu\|ContextMenu\|MouseButton::Right" \
    crates/crowbar-app/src/surfaces/tabs.rs \
    crates/crowbar-app/src/surfaces/file_tree_row.rs
(no output)
```

`tabs.rs` and `file_tree_row.rs` are the closest Rust analogs to
`tab-context-menu.tsx`'s and `use-file-explorer-context-menu.tsx`'s hosts, and
neither has any context-menu wiring, right-click handling, or native-menu
import at all. Building a menu surface now would be building a trigger for a
host component that does not exist, which is a third-hand version of the
`primitive-dialog-service` mistake this brief already warns against
("inventing an unnecessary surface is worse than an unported one, because it
creates a thing to verify") — here compounded, since the thing being
pre-emptively wired would not even be reachable from a running app.

## 5. What was not built, and why

- **No `crowbar-ui` component.** `context-menu.tsx`'s live half needs no
  design-token reproduction — it leaves the strict-parity gate the same way
  `dropdown-menu`'s did, for the same user ruling, and §2 shows its shape is
  already inside `native_menu.rs`'s existing vocabulary.
- **No new `crowbar-app` `--surface`.** `--surface native-menu` already
  demonstrates the identical vocabulary (item/separator/submenu/checked/
  disabled) against an equivalent live fixture; a second driver surface for
  `context-menu` would duplicate it under a new name for no new information,
  the same shape `tree-row.md` §"Why nothing is built here" already rejects
  for a different component.
- **No checklist additions.** The brief asks, if checklist items are added,
  for a machine-checkable/human-judged split. None are added — `--surface
  native-menu`'s existing 16-line checklist (`native_menu.rs`'s module docs)
  already covers every behaviour §2 identifies as live (items, icons via the
  *box* the checklist's row-count checks, separators, click-outside, Escape,
  choosing a row) and the one machine-checkable line it already has
  (`--open launch --dismiss-after`, line 16 of that table) is unchanged.
- **No `crowbar-platform` change.** §3's evidence argues for leaving
  `crowbar_platform::native_menu` on its existing retirement track, not for
  growing it.
- **No web edit.** The declarative half is dead code (§0) and out of scope to
  touch for a port item; the live half needs no `data-oracle-id` because it is
  never diffed.

## 6. What would change this answer

Two things, stated so a future item does not have to re-derive them:

- **If `tabs.rs`/`file_tree_row.rs` (or their eventual replacements) grow a
  real right-click handler**, the item that adds it should call
  `gpui_component::native_menu::NativeMenu` directly at that call site (per
  §3's ruling), passing `.menu_with_icon(label, icon, Box::new(action))` for
  each row — no new crate-level surface, and no new mapping file distinct
  from whatever ports `tabs`/`file-explorer` themselves.
- **If a future call site needs a checkbox, radio, or submenu row that
  `crowbar_platform::native_menu` covers and the vendored menu does not** (or
  vice versa), that is the "concrete need" `s13` asks for and should be
  raised there, by name, against that call site — not inferred from this one,
  which uses neither.
