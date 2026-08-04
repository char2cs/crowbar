//! The design system's components.
//!
//! Two of them are gate surfaces, built to be measured against their React
//! originals across the §8.3 matrix. Everything they paint comes out of
//! [`Theme`](crate::Theme) — there is no colour literal here and the build will
//! not let there be one (`scripts/check-invariants.sh` rule 4).
//!
//! * [`GitStatusRow`] — the geometry gate. Its filename and directory truncate
//!   against each other through three nested flex containers, which is where a
//!   taffy layout is most likely to disagree with `WebKit`.
//! * [`FileTreeRow`] — the **state** gate. The git row's state axis is vacuous:
//!   `SidebarTreeRow` takes an `active` prop no live consumer passes, so
//!   `data-active` never fires and every `selected` cell compares resting
//!   against resting. The file explorer row sets `data-active` on every render
//!   and lives inside `.file-tree-container`, so hover, selection **and** focus
//!   all paint something on it.
//!
//! The first Phase 2 component is [`dropdown_menu`], which is where the pattern
//! the rest of Phase 2 follows is set: one module per surface, its anchor ids and
//! its two declaration lists written down as data, its visual state a parameter,
//! and every Tailwind class translated against the app's own compiled CSS rather
//! than against the class name.
//!
//! Two conventions the rest of the components should follow:
//!
//! * **Visual state is a parameter, never a `.hover(…)` refinement.** See
//!   [`RowState`]. gpui resolves interaction refinements from runtime state
//!   that a snapshot cannot see, so a component that expresses its states that
//!   way is invisible to the oracle.
//! * **Anchors go through [`AnchorSink`]**, so the shipping build carries no
//!   oracle code and the measured build measures the same tree.

mod anchor;
mod sidebar_tree;

// Every measurable surface is a public **module**. They each carry an
// `ID_ITEM`/`ID_POPUP`, a `CONTENT_SIZED` and a `LINE_SIZED`, and those cannot
// all be flattened into one namespace — nor should they be:
// `git_status_row::ID_ITEM` and `file_tree_row::ID_ITEM` are different anchors
// on different surfaces, and code that names one should have to say which.
//
// `dropdown_menu` is flattened not at all, deliberately: every one of its public
// names would collide with something (`ID_ITEM`, `CONTENT_SIZED`, `LINE_SIZED`,
// `ICON_SIZE`, `label`), and the collisions are the *point* — a `CONTENT_SIZED`
// that silently meant the git row's would be a declaration applied to the wrong
// surface, which is precisely the mistake `ANCHORS.md` v1.6 warns about.
//
// `resizable` is unflattened for the same reason and one further one: its
// `Panel`, `Handle` and `Orientation` are short names that only read correctly
// with the module in front of them, and its `CONTENT_SIZED`/`LINE_SIZED` would
// collide exactly as `dropdown_menu`'s do.
// `button` is unflattened for the same reason as the rest, and one further one:
// its `Size` and `Variant` are short names that only read correctly with the
// module in front of them — `button::Variant::Ghost` says which component's
// vocabulary it is, where a bare `Variant` would not.
// `avatar` and `badge` are unflattened for the same reason as the rest. Each
// carries a `CallSite` — the `className` a call site merges over the variant's,
// which P3.1 established as a legitimate parameter — and the two enums name
// different bundles on different components. A bare `CallSite` would say which
// of them nowhere, and a bundle applied to the wrong primitive is the same class
// of mistake as a declaration list applied to the wrong surface.
// `alert_dialog` is unflattened for the same reason as the rest — its
// `ID_POPUP`/`ID_HEADER`/`ID_TITLE`/`ID_DESCRIPTION`/`ID_FOOTER` would collide
// with `dialog`'s outright, which matters more here than anywhere else in this
// list: the two components' *values* are the same by finding (see
// `alert_dialog.rs`'s module docs), so the module boundary is the only thing
// that keeps `alert_dialog::HEADER_PADDING` from silently reading as
// `dialog::HEADER_PADDING`'s one caller too many.
pub mod alert_dialog;
// P3.32's pair is unflattened for the same reason as the rest, and one
// further one: `autocomplete::ID_INPUT` would collide with `input`'s own
// `ID_FIELD` concept under a different name, and `autocomplete::ID_ITEM`
// with `select`'s `select-item`/`command`'s own `ID_ITEM` — three modules'
// readings of "one row in a list" that must not be confused with each
// other. `command` reuses `autocomplete`'s `Input`/`List`/`Item`/`ListContent`
// directly (see `command.rs`'s module docs for why: the two files build the
// *same* DOM nodes, restyled through a call site's `className`, not
// reimplemented), which is the one place in this list where one component
// module imports another's types rather than merely avoiding a name clash.
pub mod autocomplete;
pub mod avatar;
pub mod badge;
pub mod button;
// The two P3.10 containers are unflattened for the same reason as the rest, and
// one further one: they are the first surfaces whose anchors are *slots*, so
// every short name in them exists twice over. `ID_PANEL` is `card::ID_PANEL` (a
// `CardContent`) and `inline_error::ID_PANEL` (the whole surface) — two
// different boxes under one spelling, which is the sharpest collision in the
// tree so far. `ID_TITLE`, `CONTENT_SIZED`, `LINE_SIZED` and `PADDING` each
// exist in both, and a declaration list that silently meant the other surface's
// is the mistake `ANCHORS.md` v1.6 warns about.
pub mod card;
// The four P3.8 icon and brand primitives are unflattened for the same reason as
// the rest: each carries its own `ID_*`, `CONTENT_SIZED` and `LINE_SIZED`, and
// three of them carry a `CallSite` that names a different bundle on a different
// component. `crowbar_mark::CallSite::None` and `sidebar_toggle_icon::CallSite::None`
// are two different pictures, and a bare `CallSite` would say which of them
// nowhere. `search_toggle_icons::WEIGHT` would collide with `kbd::WEIGHT` and
// `label::WEIGHT` outright, and `sidebar_toggle_icon::EXTENT` reads as a number
// belonging to no component at all without its module in front of it.
// The two P3.9 toggles are unflattened for the same reason as the rest, and one
// further one: they are the first pair whose `selected` state is real, and each
// answers "what does `selected` move here" differently — `switch::Switch` moves
// a background *and* a thumb offset, `checkbox::Checkbox` moves one background
// and only in the dark table. `checkbox::Checked` is a three-valued enum where
// `switch`'s equivalent is a `bool`, and a bare `Checked` would say which
// component's vocabulary it is nowhere. Their `CONTENT_SIZED`/`LINE_SIZED` and
// `BORDER_WIDTH` would collide outright — `switch::BORDER_WIDTH` is 0 and
// `checkbox::BORDER_WIDTH` is 1, which is the border trap and its mirror sitting
// one module apart, and a bare `BORDER_WIDTH` meaning either of them is the
// mistake `ANCHORS.md` v1.6 warns about.
pub mod checkbox;
// `command` is unflattened for the same reason as the rest. See the note
// above `pub mod autocomplete;`: it is `autocomplete`'s one importer, reuses
// that module's structs for every box it does not build itself, and adds
// its own dialog chrome (`command-dialog-popup`), panel and footer.
pub mod command;
// `context_pill` (P3.58) is unflattened for the same reason as the rest: its
// `ID_ROOT`/`ID_TRIGGER` would sit next to every other surface's own with the
// same shape of collision, and it composes `repo_avatar::RepoAvatar` and
// `workspace_branch_icon::WorkspaceBranchIcon` directly rather than
// re-deriving either — see the module docs for why it does not call
// `button::Button::render` (the same reason `sidebar_project_header`
// doesn't).
pub mod context_pill;
pub mod crowbar_mark;
pub mod crowbar_wordmark;
// P3.21's two wraps are unflattened for the same reason as the rest, and one
// sharper one still: `dialog::ID_POPUP` and `popover::ID_POPUP` are two
// different boxes under one spelling — a real border on both, but a
// `radius_2xl` against a `radius_lg` — and `dialog::ID_FOOTER` would collide
// with a future `sheet` footer outright if either were flattened. Neither
// carries a `render(&self, theme, anchors)` like the rest: `GpuiDialog::new`
// and `GpuiSheet::new` each mint a `FocusHandle` off `cx`, which `popover`'s
// and `select`'s constructors never needed, so both take `window`/`cx`
// besides — see `dialog`'s module docs and `crowbar-app/src/surface.rs`'s
// `SurfaceParams::render_ctx` for the seam that makes a cx-less call site
// still reach them.
pub mod dialog;
// `detach_holder_modal` and `repo_import_dialog` (P3.51) are unflattened for
// the same reason as the rest, and one sharper still: both are **call sites**
// of `dialog.tsx`'s own primitive, not second React files, so the real DOM
// each paints carries `dialog-*` ids — `dialog::ID_POPUP` and this module's
// own `ID_POPUP` would collide *in fact*, not just in spelling, if this
// module were flattened, which is a sharper version of the collision
// `alert_dialog`'s own comment above already warns about. Each module's own
// namespace (`detach-holder-modal-*` / `repo-import-dialog-*`) exists only
// because `crowbar-app/src/surface.rs`'s own registry test requires every
// surface's root anchor to be unique — see either module's own doc comment
// for the finding in full. `detach_holder_modal::HEADER_PADDING_RIGHT` and
// `repo_import_dialog::HEADER_PADDING`/`HEADER_PADDING_BOTTOM` are each call
// site's own real `className` override and would read as belonging to no
// component at all without its module in front of it.
pub mod detach_holder_modal;
pub mod repo_import_dialog;
// `dropdown` is unflattened for the same reason as the rest, and one sharper
// one: it sits right next to `dropdown_menu` in this list and is easy to
// mistake for a second reading of it. It is not — see its module docs for the
// evidence — and `dropdown::ID_ROOT`/`CONTENT_SIZED`/`LINE_SIZED` would
// collide with `dropdown_menu::ID_POPUP`'s neighbours outright if flattened.
// `dropdown::BORDER_WIDTH` is `1`, the same real-border shape `popover`'s is
// and the *inverse* of `dropdown_menu`'s ring trap, which is exactly the
// mistake a bare `BORDER_WIDTH` would risk meaning here.
pub mod dropdown;
pub mod dropdown_menu;
pub mod file_tree_row;
// `fps_overlay` is unflattened for the same reason as the rest: its
// `FrameStats`/`FpsTier` read correctly only with the module in front of
// them, and its `CONTENT_SIZED`/`LINE_SIZED` would collide exactly as every
// other surface's do. P3.52 — see the module docs for why it carries no
// `data-oracle-id` at all, unlike this cluster's other two "no reference"
// members, and for [`Color::BLACK`] (`theme/token.rs`), minted for this
// component's one raw-colour inline style.
pub mod fps_overlay;
pub mod git_status_row;
// `input` is unflattened for the same reason as the rest, and one further one:
// its `Size`, `State` and `Text` are short names that only read correctly with
// the module in front of them — `button::Size` and `input::Size` are two
// different vocabularies, and a bare `Size` would not say which. Its
// `CONTENT_SIZED`/`LINE_SIZED` would collide exactly as `dropdown_menu`'s do,
// and `input::LINE_SIZED` in particular carries a *reason* the others do not.
pub mod inline_error;
pub mod input;
// The four P3.6 leaves are unflattened for the same reason as the rest: each
// carries its own `ID_*`, `CONTENT_SIZED` and `LINE_SIZED`, and three of them
// carry a `CallSite`. `kbd::CallSite` does not exist but `label::CallSite`,
// `separator::CallSite` and `skeleton::CallSite` all do, and a bare `CallSite`
// would say which of them nowhere. `separator::Orientation` would collide with
// `tabs::Orientation` outright, and `label::Label` with `git_status_row`'s
// `Label` — a bundle or a declaration list applied to the wrong primitive is the
// mistake `ANCHORS.md` v1.6 warns about.
pub mod kbd;
// `keybinding` is unflattened for the same reason as the rest, and one sharper
// one: it is the closest neighbour `kbd` has, and **every one of its box
// constants is the other's opposite**. `keybinding::BORDER_WIDTH` is 1 and
// `kbd` pays none at all; `keybinding::PADDING_X` is 6 and `kbd::PADDING_X` is
// 4; `keybinding::WEIGHT` is 400 and `kbd::WEIGHT` is 500. A bare
// `BORDER_WIDTH` meaning either of them is the border trap in both directions
// under one spelling, which is exactly the mistake `ANCHORS.md` v1.6 warns
// about. Its `Platform` and `Source` are short names that only read correctly
// with the module in front of them, and `CONTENT_SIZED`/`LINE_SIZED` would
// collide as every other surface's do.
pub mod keybinding;
pub mod label;
// `number_input` is unflattened for the same reason as the rest, and one
// further one: its `Size` and `Width` would collide with `input`'s and
// `button`'s outright — `number_input::Size` is a flat `xs`/`sm`/`md` table
// with no `sm:` breakpoint step where `button::Size` and `input::Size` both
// carry one, and confusing the three is exactly the mistake `ANCHORS.md`
// v1.6 warns about. **Built, not wrapped** — see the module docs for the
// seam test (`gpui_component::input::number_input::NumberInput` builds its
// whole `h_flex()` privately; `stepper::Stepper` is a same-named but
// unrelated wizard-progress widget) and for the two flanking buttons'
// private local copy of `button.rs`'s own glyph arithmetic.
pub mod number_input;
// The two P3.15 wraps are unflattened for the same reason as the rest, and one
// sharper one: they are the first components whose behaviour comes from
// `gpui-component`, so their vocabularies are *two* systems' at once.
// `popover::ID_POPUP` and `dropdown_menu::ID_POPUP` are two different boxes
// under one spelling — one bordered, one ringed — which is the border trap and
// its inverse sitting two modules apart. `select::Size` and `select::Breakpoint`
// would collide with `input`'s and `button`'s outright, and `select::Variant`
// with `dropdown_menu`'s `RowVariant` neighbours. A bare `BORDER_WIDTH` meaning
// `popover`'s 1, `switch`'s 0 or `checkbox`'s 1 is exactly the mistake
// `ANCHORS.md` v1.6 warns about.
//
// **`select` carries no surface**, and that is the item's finding rather than an
// omission: every box `select.tsx` styles is built inside `gpui-component` and
// `AnchorSink` takes a `Div` this crate holds. Its module docs and
// `native/mapping/select.md` carry the account.
pub mod popover;
// `radio_group` is unflattened for the same reason as the rest. Its `ID_RADIO`
// and `ID_GROUP` would sit next to `checkbox`'s and `select`'s `ID_*`
// constants with the same shape of collision, and its `CONTENT_SIZED`/
// `LINE_SIZED` would silently mean this surface's on every other module's
// behalf.
//
// **Built, not wrapped, and unreached by any parity run** — see
// `radio_group.rs`'s module docs for the seam test (`gpui_component::Radio`'s
// `ParentElement` reaches a label slot, never the circle) and for the
// reachability measurement (`radio-group.tsx`'s only importer needs a child
// branch with an unprotected local parent, and this item's dev environment
// has none).
pub mod radio_group;
// `repo_avatar` is unflattened for the same reason as the rest. Its `ID`,
// `CONTENT_SIZED` and `LINE_SIZED` would collide outright, and its `Size` —
// `sm`/`lg`/`xl`, no `md` — would read as a table belonging to no component
// without the module in front of it: `avatar::CallSite`'s three bundles
// answer a different question. P3.54 landed `data-oracle-id`s on this file
// and on `workspace_branch_icon`'s — see each module's own docs.
pub mod repo_avatar;
// `repo_icon_popover` (P3.58) is unflattened for the same reason as the rest:
// its own `ID_TRIGGER`/`ID_POPUP` and its bespoke `Kind`/`ImageState` reuse
// would read as belonging to no component without the module in front of
// them, and its trigger's own `h-5 w-5 rounded-md` box is a genuinely
// different shape from `repo_avatar::Size::Lg`'s `rounded-sm` one — see the
// module docs for why it is not built by calling `RepoAvatar::render`.
pub mod repo_icon_popover;
pub mod resizable;
// `scroll_area` is unflattened for the same reason as the rest, and one further
// one: it is the second component in the tree whose vocabulary is *two*
// systems' at once, and the one where the vendor's half was **rejected**.
// `scroll_area::Orientation` would collide with `separator`'s and `tabs`'
// outright — three components' readings of the same word, and only this one
// means "which way a scrollbar runs". `scroll_area::BORDER_WIDTH` is 0 where
// `keybinding`'s two modules over is 1, and `scroll_area::THUMB_RADIUS` is
// `f32::MAX` where `kbd::RADIUS` is 4. A bare `Overflow`, `ScrollBar` or
// `Corner` would say which component's picture it is nowhere.
pub mod scroll_area;
// `search` is unflattened for the same reason as the rest, and one further
// one: `search::ID_POPOVER`'s neighbours (`ID_CLOSE`, `ID_TOGGLE_*`,
// `ID_NAV_*`, `ID_REPLACE_*`) would sit next to every other surface's own
// `ID_*` constants with the same shape of collision, and `search::
// CONTENT_SIZED`/`LINE_SIZED` would silently mean this surface's declaration
// list on every other module's behalf — the `ANCHORS.md` v1.6 mistake this
// file has warned about since `dropdown_menu`. **Built, not wrapped** — see
// `search.rs`'s module docs: `search.tsx` renders through no `gpui-component`
// primitive at all, so every box is this crate's own to anchor and there is
// no vendor seam to test.
pub mod search;
pub mod search_toggle_icons;
pub mod select;
pub mod separator;
// `sheet` is the third P3.21 wrap, alongside `dialog` — see that module's
// comment above `pub mod dialog;` for the shared reasoning, and `sheet`'s own
// module docs for the finding that sits on top of it: no live call site
// mounts one at all, and its vendor widget cannot even be driven past
// `Placement::Right` without a `Root` this measurement harness does not mount.
pub mod sheet;
// `sidebar` is unflattened for the reason `sidebar_carousel` is, and one
// sharper: `sidebar::Header` is `sidebar.tsx`'s `SidebarHeader` and
// `card::Header` is `card.tsx`'s, two padded containers with different numbers;
// `sidebar::Tone` names three text colours where `inline_error` has its own
// notion of an error tone; and `sidebar::CONTENT_SIZED` is non-empty where the
// flattened `git_status_row::CONTENT_SIZED` already occupies the bare name — a
// declaration list that silently meant another surface's is exactly the mistake
// `ANCHORS.md` v1.6 warns about.
//
// **`sidebar` carries two surfaces and covers two of its file's six visual
// exports.** `Sidebar` and `SidebarFooter` are reported-and-stopped rather than
// rebuilt: the vendor puts geometry between its own border box and the child
// this crate can hold, so neither can reach strict parity. The module docs and
// `native/mapping/sidebar.md` carry the account and the quoted vendor code.
pub mod sidebar;
// The three P3.52 leaves are unflattened for the same reason as the rest.
// `sidebar_project_header::ID_SIDEBAR_PROJECT_HEADER` and its siblings'
// `ID_*` constants would sit next to every other surface's own with the same
// shape of collision, and `sidebar_tab_bar::Tabs` (built from
// `super::tabs::Tabs`) would read as a second, unrelated `Tabs` type without
// the module in front of it. None of the three carries a reference — see
// each module's own docs for why, and `native/mapping/layout-denominator.md`
// §8 Cluster 3 for the survey that grouped them.
pub mod sidebar_project_header;
pub mod sidebar_skeleton;
pub mod sidebar_tab_bar;
pub mod sidebar_toggle_icon;
pub mod skeleton;
// `slider` is unflattened for the same reason as the rest, and one sharper:
// it is the second genuinely style-only widget (`switch` the first) and the
// closest neighbour `switch` has. `slider::ID_THUMB` would collide with
// `switch::ID_THUMB` outright — two different knobs, two different colour
// rules (`switch`'s thumb moves nothing across themes; `slider`'s moves only
// its border) — and `slider::ROUNDED_FULL` restates `switch::TRACK_RADIUS` and
// `scroll_area::THUMB_RADIUS` under a third name, which is the point: three
// surfaces independently measuring the same `f32::MAX` trap should not share
// one constant that a change to any of them could silently move for the
// others. `slider::Slider::thumb_center` in particular reads as a number
// belonging to no component at all without its module in front of it.
pub mod slider;

// `tooltip` is unflattened for the same reason as the rest. Its `ID_TOOLTIP`
// would sit next to `select`'s and `popover`'s `ID_*` constants with the same
// shape of collision, and its `CONTENT_SIZED`/`LINE_SIZED` would silently mean
// this surface's on every other module's behalf — the `ANCHORS.md` v1.6
// mistake this file has warned about since `dropdown_menu`.
//
// **Built, not wrapped** — the item's finding, and the inverse of `popover`'s.
// `gpui_component::tooltip::Tooltip` paints its whole box inside a private
// `Render::render`; the only public seam is `Styled::style()`, a
// `StyleRefinement` on a `Div` this crate never holds, which is the exact
// "nothing to anchor" shape `popover`'s own module docs and this item's brief
// both name as the fake convergence `ANCHORS.md` exists to refuse. See
// `tooltip.rs`'s module docs for the seam test applied in full, and for why
// `tooltip.tsx` is a distinct surface from `popover`'s unreached
// `tooltipStyle` arm rather than a second reading of it.
pub mod tooltip;
// The three P3.7 spinners are unflattened for the same reason as the rest, and
// one further one: they are a *family*, so every short name in them exists three
// times over. `CallSite` names three different className bundles;
// `spinner::SIZE_4` and `flicker_spinner::SIZE_4` are two components' readings
// of the same utility and only one of them is the captured cell; and
// `spinner::PERIOD` against `flicker_spinner::FLICKER_PERIOD_RANGE` is the
// distinction the whole item turns on — one animation moves a recorded field and
// the other does not. A bare `CallSite` or a bare `SIZE_4` would say which of
// them nowhere, and `loading_spinner::CONTENT_SIZED` in particular is the only
// non-empty one of the three.
pub mod flicker_spinner;
pub mod loading_spinner;
pub mod spinner;
// `sidebar_carousel` is flattened not at all for the same reason: its
// `CONTENT_SIZED`, `LINE_SIZED` and `ID_*` are its own, and a declaration list
// that silently meant another surface's is the mistake `ANCHORS.md` v1.6 warns
// about.
pub mod sidebar_carousel;
// `switch` — see the note above `checkbox`.
pub mod switch;
// `tabs` is unflattened for the third time and the same reason: its
// `Orientation`, `Variant`, `Panel`, `Tab`, `CONTENT_SIZED` and `LINE_SIZED`
// would each collide with another surface's, and a declaration list that
// silently meant `resizable`'s is the mistake `ANCHORS.md` v1.6 warns about.
// `Panel` is the sharpest of them: `resizable::Panel` is a resize pane and
// `tabs::Panel` is a tab's content, and nothing about the name says which.
pub mod tabs;
// `textarea` is unflattened for the same reason as the rest, and one further
// one: its `Size` would collide with `input`'s, `button`'s and
// `number_input`'s outright, each a different table. **Built, not wrapped**
// — `gpui_component::input::Input`'s multi-line mode builds its whole
// editable text element privately (`input/element.rs`), the same shape
// `input.rs` already found for the single-line case — and **unreached by any
// parity run**: `textarea.tsx`'s only importer, `commit-popover.tsx`, sits
// behind a git panel whose "Changes" list would not populate in this item's
// dev environment even after a real `git/stage` API call and a full reload,
// confirmed against the backend's own `git/status` response showing six
// real dirty files. The module docs and `native/mapping/textarea.md` carry
// the account and the measurement technique used without a live mount.
pub mod textarea;
// `toast` is unflattened for the same reason as the rest, and one further one:
// P3.28's own finding is that it shares nothing with `popover`'s unreached
// `Variant::Tooltip` but a name (see `toast.rs`'s module docs), so a bare
// `CONTENT_SIZED`/`LINE_SIZED` here would be exactly the "silently means
// another surface's" mistake `ANCHORS.md` v1.6 warns about, one door over from
// `tooltip`'s own version of the same warning.
//
// **Built, not wrapped, and unreached by any live producer** — see
// `toast.rs`'s module docs for the seam test (`gpui_component::Notification`'s
// only element-shaped seam is a `'static` closure, unreachable from
// `&dyn AnchorSink`'s anonymous lifetime — `popover`'s own trap, and this
// primitive has none of `Popover`'s `ParentElement` fallback either) and for
// the reachability finding (`anchoredToastManager.add` — the only thing that
// can put a toast in `toast.tsx`'s own render path — has zero call sites
// anywhere in `web/src`).
pub mod toast;
// `workspace_branch_icon` is unflattened for the same reason as the rest. Its
// `Status`, `Glyph` and `ID` would collide with `file_tree_row`'s `GitStatus`
// and `git_status_row`'s `ID_ICON` neighbours under a different shape of the
// same mistake, and it reuses `flicker_spinner::CallSite::WorkspaceIcon`
// directly rather than reimplementing the spinner it wraps — see its module
// docs.
pub mod workspace_branch_icon;
// `row_base` carries no anchor of its own — no `data-oracle-id` anywhere in
// `workspace-row-base.ts`, and it paints nothing by itself — so it is not a
// surface. It is `pub`, unlike `anchor`/`sidebar_tree` above: this crate's
// own [`project_home_row`]/[`project_switcher_panel`] reach it as siblings
// either way, but `crowbar-app`'s `row_layout` tests for both also assert
// against its constants directly (`row_base::HEIGHT`,
// `row_base::MARGIN_Y`, …) to pin the real taffy layout against the same
// numbers the components themselves were built from, rather than against a
// second, hand-copied literal — the same reason `git_status_row`'s own
// constants are `pub` and reached the same way from that crate's tests.
pub mod row_base;
// `project_home_row` (P3.60) is unflattened for the same reason as the
// rest — its `ID_ROOT`/`ID_ICON`/`ID_LABEL` would collide with other
// surfaces' generic-sounding names under a different shape of the same
// mistake. Composes `workspace_branch_icon` directly (never reimplements
// it) and hand-builds its own two trailing-action buttons off
// `button::Size`/`RadiusClass` rather than nesting `Button::render`'s own
// anchor machinery — `context_pill.rs`'s and `sidebar_project_header.rs`'s
// precedent. See its own module docs.
pub mod project_home_row;
// `project_switcher_panel` (P3.60) is unflattened for the same reason.
// `project-home-row.tsx` pushes this panel onto the nav stack on click, so
// it lands alongside it — see `native/mapping/layout-denominator.md` §8's
// Cluster 5. Its own row ids are index-parameterized, not fixed strings;
// see its module docs for why.
pub mod project_switcher_panel;
// `workspace_switcher` (P3.58) is unflattened for the same reason as the
// rest, and carries **no anchor of its own** — see the module docs. Its
// `Row` reuses `autocomplete::ID_ITEM` directly (the one real, already-
// registered anchor for a `command-item`'s content, which `autocomplete.rs`
// itself leaves opaque) rather than fabricating a second id nothing on the
// React side would ever produce.
pub mod workspace_switcher;
// `nav_stack` (P3.59) is unflattened for the same reason as the rest — its
// `Screen` would collide with a hypothetical future primitive under the same
// short name — and reuses `sidebar_project_header::HEIGHT_MAC`/
// `HEIGHT_OTHER`/`TRAFFIC_LIGHTS_WIDTH` directly rather than re-deriving
// them: `nav-stack.tsx`'s own comment ties its header's height and padding
// to `SidebarProjectHeader`'s. See the module docs.
pub mod nav_stack;
// `sidebar_peek` (P3.59) is unflattened for the same reason as the rest.
// Carries one anchor, on its *inner* div — see the module docs for why the
// outer `data-sidebar-peek` wrapper gets none, the same call
// `workspace_switcher`'s own wrapper made, extended to a conditionally
// `display: contents` element.
pub mod sidebar_peek;
// `workspace_inline_input` (P3.62) is unflattened for the same reason as the
// rest — its `ID_ROOT`/`ID_FIELD`/`Kind` would read ambiguously without the
// module in front of them. Its `<input>` field is `input.rs`'s own "box only,
// no text field" finding one door over; see the module docs.
pub mod workspace_inline_input;
// `placeholder_row_actions` (P3.62) is unflattened for the same reason as the
// rest, and one further one: `ID_RETRY` is a call-site rename of `button`'s
// own default id, and a bare `ID_RETRY` next to `inline_error::ID_RETRY`
// (the identical rename, a different surface) would be exactly the collision
// this file's own docs warn a flattened namespace produces.
pub mod placeholder_row_actions;
// `sidebar_toast_overlay` (P3.62) is unflattened for the same reason as the
// rest, and one further one: it backs **two** registered surfaces
// (`sidebar-toast-overlay`/`sidebar-toast-overlay-fallback`, the registry's
// unique-root constraint applied to one component's two DOM shapes — see the
// module docs §1), and both call sites need `Kind`/`Side`/`ToastFixture`
// read as this module's own.
pub mod sidebar_toast_overlay;

pub use anchor::{AnchorId, AnchorSink, Unanchored};
pub use avatar::Avatar;
pub use badge::Badge;
pub use button::Button;
pub use dropdown_menu::DropdownMenu;
pub use input::Input;
pub use resizable::ResizablePanelGroup;
pub use tabs::Tabs;
// `GitStatus` and its vocabulary are flattened because nothing on the other
// surface is called that: the git status row's filename is pinned at
// `text-foreground` and never takes a status colour.
pub use file_tree_row::{ALL_GIT_STATUSES, FileTreeRow, GitStatus};
pub use git_status_row::{
    BADGE_LABEL, BREAKPOINT_SM, Breakpoint, CONTENT_SIZED, ContentLength, GitStatusRow, ID_ADDED,
    ID_BADGE, ID_BUTTON, ID_DELETED, ID_DIR, ID_ICON, ID_ITEM, ID_NAME, LINE_SIZED,
    NAME_MAX_FRACTION, NameSizing, TrailingContent, guide_id, name_sizing, shows_directory_span,
    split_path,
};
pub use sidebar_tree::{
    BASE_INDENT, GUIDE_END_INSET, GUIDE_OPACITY_PERCENT, GUIDE_RULE_INSET, GUIDE_RULE_WIDTH,
    GUIDE_SHIFT, GUIDE_WIDTH, GuideInset, ICON_SIZE, INDENT_SIZE, ITEM_RADIUS, ROW_HEIGHT,
    RowState, guide, guide_at, guide_inset, guide_left, item, item_background, leading_padding,
    row_button,
};
