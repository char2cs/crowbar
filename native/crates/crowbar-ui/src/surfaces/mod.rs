//! The design system's **surfaces** — compositions that know what a repo, a
//! workspace, a project or the Crowbar sidebar is, built out of
//! [`crate::primitives`]. S0.4's split of the former flat `components/`
//! directory; see `crate::primitives`'s own module docs for the split's
//! rationale and its one crate-root exception (`crate::anchor`).
//!
//! Grouped where the grouping is obvious from the name
//! (`sidebar`/`workspace`/`repo`), plus one more, [`rows`], for the shared
//! row family that composes across those groups (`row_base`,
//! `git_status_row`, `file_tree_row`, `pending_create_row`,
//! `placeholder_row_actions`, `project_home_row`, `project_switcher_panel`,
//! and `sidebar_tree` — see [`rows`]'s own module docs for why the last one
//! sits there despite its name). Everything left over sits flat here,
//! because a further group would be a group of one or two and "prefer
//! obvious over clever" (the item's own brief) argues against inventing one.
//!
//! **Borderline calls (item's brief, §"genuinely arguable"), decided here:**
//!
//! * [`search`] and [`search_toggle_icons`] — the editor/terminal
//!   find-and-replace shell. Its anchors (`search-replace-toggle`,
//!   `search-nav-previous`, …) and its module docs both frame it as chrome
//!   for Crowbar's own editor and terminal panes, not a reusable UI-kit
//!   widget a different application would ship unchanged — nothing about a
//!   generic find/replace bar requires *this* app's specific editor+terminal
//!   pairing. Filed as a surface.
//!
//! See [`rows`]'s own module docs for the other two the brief named
//! (`row_base`, `nav_stack` lives in [`sidebar`] instead — see its own note).

pub mod repo;
pub mod rows;
pub mod sidebar;
pub mod workspace;

// `context_pill` (P3.58) is unflattened for the same reason as the rest: its
// `ID_ROOT`/`ID_TRIGGER` would sit next to every other surface's own with the
// same shape of collision, and it composes `repo::repo_avatar::RepoAvatar` and
// `workspace::workspace_branch_icon::WorkspaceBranchIcon` directly rather than
// re-deriving either — see the module docs for why it does not call
// `crate::primitives::button::Button::render` (the same reason
// `sidebar::sidebar_project_header` doesn't).
pub mod context_pill;
// The four P3.8 icon and brand primitives — `crowbar_mark`, `crowbar_wordmark`,
// `search_toggle_icons` here and `sidebar::sidebar_toggle_icon` one module
// over — are unflattened for the same reason as the rest: each carries its
// own `ID_*`, `CONTENT_SIZED` and `LINE_SIZED`, and three of them carry a
// `CallSite`. `crowbar_mark::CallSite::None` and
// `sidebar::sidebar_toggle_icon::CallSite::None` are two different pictures,
// and a bare `CallSite` would say which of them nowhere. `search_toggle_icons
// ::WEIGHT` would collide with `crate::primitives::kbd::WEIGHT` and
// `crate::primitives::label::WEIGHT` outright, and
// `sidebar::sidebar_toggle_icon::EXTENT` reads as a number belonging to no
// component at all without its module in front of it.
pub mod crowbar_mark;
pub mod crowbar_wordmark;
// `detach_holder_modal` (P3.51) is unflattened for the same reason as the
// rest, and one sharper still: it is a **call site** of `dialog.tsx`'s own
// primitive, not a distinct React file — see its own module docs and
// `repo::repo_import_dialog`'s (this cluster's other half, one directory
// over because of the `repo_` prefix) for the shared finding: the real DOM
// each paints carries `dialog-*` ids, not a namespace of its own, and each
// module's own anchor namespace exists only because
// `crowbar-app/src/surface.rs`'s own registry test requires every surface's
// root anchor to be unique.
pub mod detach_holder_modal;
// `search` (P3.33) and `search_toggle_icons` are unflattened for the same
// reason as the rest. `search::ID_POPOVER`'s neighbours (`ID_CLOSE`,
// `ID_TOGGLE_*`, `ID_NAV_*`, `ID_REPLACE_*`) would sit next to every other
// surface's own `ID_*` constants with the same shape of collision, and
// `search::CONTENT_SIZED`/`LINE_SIZED` would silently mean this surface's
// declaration list on every other module's behalf. **Built, not wrapped** —
// `search.tsx` renders through no `gpui-component` primitive at all, so
// every box is this crate's own to anchor and there is no vendor seam to
// test. See the module-level doc comment above for why this pair is a
// surface rather than a primitive.
pub mod search;
pub mod search_toggle_icons;
