# `workspace-switcher` (P3.58)

`web/src/components/layout/workspace-switcher.tsx` →
`crates/crowbar-ui/src/components/workspace_switcher.rs`.

**No `--surface`, no `crates/crowbar-app/src/surfaces/workspace_switcher.rs`
— an argued exception, not an omission.** See §3. Geometry coverage instead
lives in `crates/crowbar-app/src/row_layout/workspace_switcher.rs`, driven
directly through the shared harness the `sidebar-tab-bar.md` shape
established.

**No live reference.** This item does not run the oracle or capture a
snapshot. `command.rs`'s own module docs already carry a live capture of
this call site's outer chrome (*"the live workspace switcher, reached from
the Context Pill"*) — see §1 for what that capture does and does not cover.

## 0. What is new here, and what already existed

`workspace-switcher.tsx` renders `<Command>` (the searchable list dialog
content) inside `context-pill.tsx`'s `CommandDialogPopup`. The chrome —
popup, panel, footer, input row — is `command.tsx`'s own boxes, and
`crowbar_ui::components::command`'s existing port already models all of it,
using generic `autocomplete::Item`/`List` fixtures for the one row this dev
environment's fixture holds. What that generic model deliberately leaves
opaque is the row's own **content**: `autocomplete::Item::content_height`'s
own doc comment says so outright — *"none of that is `autocomplete.tsx`'s"*.
This item's whole job is that content: the identity glyph, the two-part
label, the optional change counts, the optional check — modelled here as
`Row`, and composed into the *existing* anchored box rather than a new one.

## 1. `command.rs`'s own reference already confirms this row's height

`command.rs`'s own fixture (`Command::fixture`) documents the live cell's
list as `574×46` with one highlighted item, and `autocomplete::
Item::fixture` documents that item's own `content_height` as `18` — *"a
13px label's own line box"*. That capture predates this item and used
`autocomplete::Item`'s generic empty box, not `Row`'s real content, but the
**height** it recorded is exactly the number `Row::shell` reproduces
(`command::ITEM_PADDING_Y × 2 + 18 = 30px`, `row_layout::
workspace_switcher::the_fixture_row_is_30px_tall` holds it through a real
taffy layout) — this item does not re-measure that number, it gives it real
content.

## 2. Values

| React / Tailwind | Compiles to | gpui / `crowbar-ui` |
|---|---|---|
| `gap-2` (row) | 8px | `GAP` |
| `py-1.5` (row, `command.tsx`'s override) | 6px | reused: `command::ITEM_PADDING_Y` |
| `px-2` (row, `autocomplete.tsx`'s own) | 8px | reused: `autocomplete::ITEM_PADDING_X` |
| `rounded-sm` (row) | 6px | reused: `autocomplete::ITEM_RADIUS` |
| `text-[13px]` (label) | 13px font, 18px line — see `context-pill.md` §2 for the font-family identity that lets this number transfer | `LABEL_TEXT` / `CONTENT_HEIGHT` |
| `<Library size={14}>` (home glyph) | 14px, unanchored | `LIBRARY_SIZE` |
| `<Check>` (no size prop) | no compiled number exists — see the component's own module docs | `CHECK_SIZE`, modelled at 16px to match the sibling status icon, flagged as an assumption |
| `text-green-300`/`text-red-300` (counts) | Tailwind's own oklch, no token coincides | `Color::GREEN_300`/`Color::RED_300` (`crowbar-ui/src/theme/token.rs`, added by this item) |

## 3. Why this stays a `crowbar-ui` module rather than becoming its own `--surface`

`workspace-switcher.tsx`'s own outer `<div ref={rootRef}
className="contents">` is CSS `display: contents` — it generates **no box
at all**, so there is nothing for a wrapper anchor to name even in
principle (sharper than `sidebar-tab-bar.tsx`'s own reason: that wrapper
*could* carry a box and chose not to anchor it; this one cannot generate one
regardless). Every real anchor a rendered menu carries belongs to a
different file: `command.tsx`'s own chrome, `autocomplete.tsx`'s own
`autocomplete-item` (`AutocompleteItem`'s own default, never overridden by
`CommandItem`), or `repo-avatar.tsx`/`workspace-branch-icon.tsx`'s own ids
nested inside it.

`surface.rs`'s own registry forbids two surfaces sharing a root
(`every_registered_surface_has_its_own_name_and_root`), and reusing
`command`'s own `command-dialog-popup` would be exactly that; minting a
`workspace-switcher-item` nobody's capture would ever produce is the
fabricated-anchor move `ANCHORS.md` refuses. `Row::render` therefore opts
into `autocomplete::ID_ITEM` directly (`"autocomplete-item"` — the one real
id already registered for this exact element) rather than either.

It does not cost oracle coverage: the item's own height and structure are
already reachable through `--surface command`'s existing cell, unchanged by
this item; `Row`'s own content composition — which glyph, which label
halves, which counts, which check — is what `row_layout/workspace_
switcher.rs` measures directly, the `sidebar_tab_bar.rs` precedent for a
"no root of its own" composition applied a second time.

## 4. React: no new `data-oracle-id`, and why

`workspace-switcher.tsx` gets **no new id** from this item — the honest
deliverable here is the comment (in the file, above `WorkspaceSwitcherMenu`)
rather than an anchor, `sidebar-tab-bar.tsx`'s own precedent. Every anchor
the file's content reaches is already someone else's: `command.tsx`'s own
chrome ids, `AutocompleteItem`'s own unconditional default, and
`repo-avatar`/`workspace-branch-icon`'s own ids (P3.54) nested inside via
`<RepoAvatar>`/`<WorkspaceBranchIcon>`. No `oracleSurfaceScope` entry is
needed either, because this file has no root of its own to declare a set
*for* — the existing `command` scope entry already excludes
`autocomplete-item` for its own, independent reason (a real, capturable-in-
isolation row whose *count* is a cell property, `select-item`'s own
reasoning).

## 5. Composes `repo_avatar`/`workspace_branch_icon`, does not reimplement them

`Row::glyph` reaches `repo_avatar::RepoAvatar::render`/`workspace_branch_
icon::WorkspaceBranchIcon::render` directly — both opt their own anchor via
`.boxed()`, never `.root()`, so nesting them inside `autocomplete::ID_ITEM`'s
own box carries no collision risk. No `oracleSurfaceScope` entry needed for
either: the nested `repo-avatar`/`workspace-branch-icon` (and, on the
`working` branch, `flicker-spinner`) is exactly what this composition
paints, not foreign content left unpainted the way `sidebar-project-
header`'s toggle icon is — the identical finding those two modules' own
docs record from their side.

## 6. Declarations

`CONTENT_SIZED = []`. `LINE_SIZED = []` — the row's own box is authored
padding around content, never a bare text run's line box exposed as the
anchor's own height (the anchor is the row, not the label).

## 7. `row_layout` coverage

* `Row::render` never opts a second, fabricated root in — the one anchor it
  carries is `autocomplete::ID_ITEM`, reused
* the fixture row is 30px tall, matching `command.rs`'s own pre-existing
  reference for this exact call site (§1)
* a working, avatar-bearing workspace row shows the spinner, not the avatar
* `is_current` adds no anchor — `<Check>` carries none in the source, so a
  current row's own anchor set is identical to a non-current one's
* a home row's own anchor set is `autocomplete::ID_ITEM` alone —
  `Library`/`Check` are both unanchored, unlike a workspace row's status
  glyph

## 8. Reachability

`context-pill.tsx`'s `CommandDialogPopup`, the one call site — confirmed in
`native/mapping/liveness-audit.md`.
