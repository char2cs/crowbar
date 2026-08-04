//! The design system's **primitives** — generic widgets that carry no
//! knowledge of a repo, a workspace, a project or the Crowbar sidebar. Each
//! would sit unchanged in a UI kit for a completely different application.
//! Crowbar-specific compositions of these live in [`crate::surfaces`].
//!
//! S0.4 split this out of the former flat `components/` directory
//! (`crowbar-ui/src/components/*.rs`, 68 files plus `mod.rs`) into
//! `primitives/` and `surfaces/`. The move was `git mv` plus the `use`-path
//! fixups that followed from it — no logic changed, no new component was
//! added. See `surfaces/mod.rs` for the surface half of the split, and the
//! commit message for the classification this item made, including the four
//! genuinely arguable calls the item's brief asked for by name
//! (`row_base`, `nav_stack`, `search`, `fps_overlay`) and one it found on its
//! own (`anchor`, at the crate root rather than in either bucket — see its
//! own note below).
//!
//! Two of them are gate surfaces, built to be measured against their React
//! originals across the §8.3 matrix. Everything they paint comes out of
//! [`Theme`](crate::Theme) — there is no colour literal here and the build will
//! not let there be one (`scripts/check-invariants.sh` rule 4).
//!
//! * [`GitStatusRow`](crate::surfaces::rows::git_status_row::GitStatusRow) —
//!   the geometry gate, in [`crate::surfaces::rows`] rather than here: it
//!   composes and anchors real git-status rows, which is Crowbar's own
//!   concept, even though (see below) several primitives in this module
//!   depend back on its `Breakpoint` type.
//! * [`FileTreeRow`](crate::surfaces::rows::file_tree_row::FileTreeRow) —
//!   likewise a surface, in [`crate::surfaces::rows`].
//!
//! # `git_status_row::Breakpoint` — a primitive concept stranded in a surface
//!
//! Not a call this item gets to make silently: a large fraction of the
//! modules below (`badge`, `button`, `checkbox`, `inline_error`, `input`,
//! `kbd`'s neighbour `keybinding`, `label`, `loading_spinner`, `number_input`,
//! `popover`'s neighbour `radio_group`, `slider`, `spinner`, `switch`,
//! `tabs`, and more besides) import `Breakpoint` — a bare `sm:`-vs-default
//! responsive selector with no git-status meaning whatsoever — from
//! [`crate::surfaces::rows::git_status_row`]. `git_status_row.rs` is on the
//! task's own "clear surfaces" list, and correctly so: the row it renders is
//! entirely Crowbar's. But the `Breakpoint` *type* it happens to define is
//! not — it is exactly the kind of generic, would-exist-in-any-UI-kit
//! concept this module's own test asks about, just defined in the wrong
//! place by an earlier item. Moving the type would be a logic change (a
//! second file, a re-export, or a signature change propagating through every
//! one of its call sites) that this item's brief rules out — "no logic
//! changes… If you find yourself editing the body of a component, stop —
//! that is out of scope." So the dependency edge stays exactly as it was,
//! now spelled `crate::surfaces::rows::git_status_row::Breakpoint`, and is
//! recorded here as a finding for whichever future slice next touches either
//! side of it: a primitives-side `Breakpoint` (or `breakpoint` primitive
//! module) is the natural fix, not attempted here.
//!
//! # `anchor` — neither primitive nor surface, kept at the crate root
//!
//! [`crate::anchor`] (`AnchorId`, `AnchorSink`, `Unanchored`) is not a
//! component at all — it paints nothing by itself — and is used identically
//! by every module in both `primitives/` and `surfaces/`. Filing it under
//! either bucket would misrepresent it (it carries no widget a foreign UI kit
//! would want, and no Crowbar domain knowledge either), so it sits beside
//! `primitives/`, `surfaces/` and `theme/` as its own top-level module, the
//! same way `theme/` already does. This is the item's own fifth
//! classification call, alongside the four the brief named.
//!
//! Two conventions the rest of the components should follow:
//!
//! * **Visual state is a parameter, never a `.hover(…)` refinement.** See
//!   [`RowState`](crate::surfaces::RowState). gpui resolves interaction
//!   refinements from runtime state that a snapshot cannot see, so a
//!   component that expresses its states that way is invisible to the
//!   oracle.
//! * **Anchors go through [`AnchorSink`](crate::anchor::AnchorSink)**, so the
//!   shipping build carries no oracle code and the measured build measures
//!   the same tree.

// Every measurable surface is a public **module**. They each carry an
// `ID_ITEM`/`ID_POPUP`, a `CONTENT_SIZED` and a `LINE_SIZED`, and those cannot
// all be flattened into one namespace — nor should they be: two primitives'
// `ID_ITEM`s are different anchors on different surfaces, and code that names
// one should have to say which.
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
// `fps_overlay` (P3.52) is unflattened for the same reason as the rest: its
// `FrameStats`/`FpsTier` read correctly only with the module in front of
// them, and its `CONTENT_SIZED`/`LINE_SIZED` would collide exactly as every
// other module's do. See its own module docs for why it carries no
// `data-oracle-id` at all, and for [`Color::BLACK`](crate::theme::Color)
// (`theme/token.rs`), minted for this component's one raw-colour inline
// style.
//
// **Borderline call (item's brief, §"genuinely arguable"), decided here.**
// `fps_overlay` reads settings and paints frame/FPS numbers; it names no
// repo, workspace, project or sidebar concept anywhere in its source — the
// discriminator the brief gives — so by that test it is a primitive, even
// though "a UI kit for a completely different application" is an odd frame
// for a dev instrument. It is grouped with the other self-contained
// leaves (`flicker_spinner`, `loading_spinner`, `spinner`) rather than with
// `crate::surfaces`, which is reserved for things that know Crowbar's
// business objects.
pub mod flicker_spinner;
pub mod fps_overlay;
pub mod loading_spinner;
pub mod spinner;
// The two P3.10 containers' neighbour: `inline_error` is unflattened for the
// same reason as `card` above, and shares its `ID_PANEL`/`ID_TITLE`/
// `CONTENT_SIZED`/`LINE_SIZED`/`PADDING` collision with it — see `card`'s own
// comment. Its only call site (`workspace-tree.tsx`) is Crowbar's, but the
// component itself — a generic retry panel — is not; nothing in its source
// names a repo, workspace, project or the sidebar.
pub mod inline_error;
// `input` is unflattened for the same reason as the rest, and one further one:
// its `Size`, `State` and `Text` are short names that only read correctly with
// the module in front of them — `button::Size` and `input::Size` are two
// different vocabularies, and a bare `Size` would not say which. Its
// `CONTENT_SIZED`/`LINE_SIZED` would collide exactly as `dropdown_menu`'s do,
// and `input::LINE_SIZED` in particular carries a *reason* the others do not.
pub mod input;
// The four P3.6 leaves are unflattened for the same reason as the rest: each
// carries its own `ID_*`, `CONTENT_SIZED` and `LINE_SIZED`, and three of them
// carry a `CallSite`. `kbd::CallSite` does not exist but `label::CallSite`,
// `separator::CallSite` and `skeleton::CallSite` all do, and a bare `CallSite`
// would say which of them nowhere. `separator::Orientation` would collide with
// `tabs::Orientation` outright, and `label::Label` with a git-status row's own
// `Label` — a bundle or a declaration list applied to the wrong primitive is
// the mistake `ANCHORS.md` v1.6 warns about.
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
pub mod select;
pub mod separator;
// `sheet` is the third P3.21 wrap, alongside `dialog` — see that module's
// comment above `pub mod dialog;` for the shared reasoning, and `sheet`'s own
// module docs for the finding that sits on top of it: no live call site
// mounts one at all, and its vendor widget cannot even be driven past
// `Placement::Right` without a `Root` this measurement harness does not mount.
pub mod sheet;
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
// `switch` — see the note above `checkbox`.
pub mod switch;
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

pub use avatar::Avatar;
pub use badge::Badge;
pub use button::Button;
pub use dropdown_menu::DropdownMenu;
pub use input::Input;
pub use resizable::ResizablePanelGroup;
pub use tabs::Tabs;
