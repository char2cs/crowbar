//! The row family: the shared chrome every list row composes
//! ([`row_base`]), the two measured gate surfaces
//! ([`git_status_row::GitStatusRow`] and [`file_tree_row::FileTreeRow`]),
//! the tree-guide primitives they both render through ([`sidebar_tree`]),
//! and the workspace-creation/error rows built on top of all three
//! ([`pending_create_row`], [`placeholder_row_actions`]), plus the two
//! project-list rows ([`project_home_row`], [`project_switcher_panel`]).
//!
//! Two of these are gate surfaces, built to be measured against their React
//! originals across the §8.3 matrix. Everything they paint comes out of
//! [`Theme`](crate::Theme) — there is no colour literal here and the build
//! will not let there be one (`scripts/check-invariants.sh` rule 4).
//!
//! * [`git_status_row::GitStatusRow`] — the geometry gate. Its filename and
//!   directory truncate against each other through three nested flex
//!   containers, which is where a taffy layout is most likely to disagree
//!   with `WebKit`.
//! * [`file_tree_row::FileTreeRow`] — the **state** gate. The git row's state
//!   axis is vacuous: `SidebarTreeRow` takes an `active` prop no live
//!   consumer passes, so `data-active` never fires and every `selected` cell
//!   compares resting against resting. The file explorer row sets
//!   `data-active` on every render and lives inside `.file-tree-container`,
//!   so hover, selection **and** focus all paint something on it.
//!
//! # `sidebar_tree` is grouped here, not in `super::sidebar`, on the strength
//! # of its actual dependency graph rather than its name
//!
//! Its name matches the `sidebar_*` prefix `super::sidebar` groups by, but
//! nothing in `super::sidebar` imports it — only [`git_status_row`] and
//! [`file_tree_row`], both here, do (`use super::sidebar_tree::{…}`, unchanged
//! by the move because all three are still siblings). Grouping by name would
//! have split a private module from its only two consumers across a privacy
//! boundary it cannot cross — `sidebar_tree` carries no anchor of its own
//! ([`RowState`] and the rest are declared `pub` only so
//! `crowbar-app`'s row-layout tests can pin the same taffy numbers the
//! components were built from) and stays `mod`, not `pub mod`, exactly as it
//! was in the flat directory.
//!
//! # `row_base` — a borderline call (item's brief, §"genuinely arguable")
//!
//! [`row_base`] carries no anchor of its own and paints nothing by itself —
//! see its own module docs. Mechanically it is exactly the kind of thing a
//! generic UI kit might ship (height/padding/gap constants and a shell
//! builder), which is the case *for* `primitives/`. But it exists for one
//! reason only: so [`project_home_row`], `workspace_tree_item`, `repo_section`
//! and [`pending_create_row`] read the *same* numbers a **sidebar row**
//! composes — its own module doc calls it "the shared row chrome every
//! sidebar row composes," and every one of its consumers is a surface in
//! this module or a neighbouring one. A foreign application's UI kit would
//! not ship "the chrome every Crowbar sidebar row composes" unchanged; it
//! would ship a generic list-row primitive with none of this file's own
//! specific numbers pinned to this app's rows. Filed as a surface, grouped
//! here with its consumers rather than flat, because "obvious over clever"
//! favours locality with the family it exists to serve.
//!
//! # The P3.61 chain
//!
//! `pending_create_row` → `super::workspace::workspace_tree_item` →
//! `super::repo::repo_section` → `super::workspace::workspace_tree` is
//! `native/mapping/layout-denominator.md` §8's Cluster 8 — a genuine
//! dependency chain, landed together by one item, that crosses the
//! `rows`/`workspace`/`repo` boundary this split draws. Each is unflattened
//! for the usual reason (`ID_ROOT`/`ID_LABEL` would collide with other
//! surfaces' generic-sounding names otherwise). Only `workspace_tree` calls
//! `AnchorSink::root`; the other three are always composed — by a parent, or
//! by `workspace_tree_item` itself, recursively — and use `AnchorSink::boxed`
//! for their own root, `workspace_branch_icon`'s own precedent. See each
//! module's own docs.

// `row_base` carries no anchor of its own — no `data-oracle-id` anywhere in
// `workspace-row-base.ts`, and it paints nothing by itself — so it is not a
// surface in the measurable sense. It is `pub`, unlike `sidebar_tree` below:
// this crate's own `project_home_row`/`project_switcher_panel` reach it as
// siblings either way, but `crowbar-app`'s `row_layout` tests for both also
// assert against its constants directly (`row_base::HEIGHT`,
// `row_base::MARGIN_Y`, …) to pin the real taffy layout against the same
// numbers the components themselves were built from, rather than against a
// second, hand-copied literal — the same reason `git_status_row`'s own
// constants are `pub` and reached the same way from that crate's tests.
pub mod row_base;

mod sidebar_tree;

pub mod file_tree_row;
pub mod git_status_row;

// `pending_create_row` → `workspace_tree_item` → `repo_section` →
// `workspace_tree` (P3.61) — see the module-level doc comment above for the
// chain in full.
pub mod pending_create_row;
// `placeholder_row_actions` (P3.62) is unflattened for the same reason as the
// rest, and one further one: `ID_RETRY` is a call-site rename of
// `crate::primitives::button`'s own default id, and a bare `ID_RETRY` next
// to `crate::primitives::inline_error::ID_RETRY` (the identical rename, a
// different surface) would be exactly the collision this file's own docs
// warn a flattened namespace produces.
pub mod placeholder_row_actions;
// `project_home_row` (P3.60) is unflattened for the same reason as the
// rest — its `ID_ROOT`/`ID_ICON`/`ID_LABEL` would collide with other
// surfaces' generic-sounding names under a different shape of the same
// mistake. Composes `super::workspace::workspace_branch_icon` directly
// (never reimplements it) and hand-builds its own two trailing-action
// buttons off `crate::primitives::button::Size`/`RadiusClass` rather than
// nesting `Button::render`'s own anchor machinery — `super::context_pill.rs`'s
// and `super::sidebar::sidebar_project_header.rs`'s precedent. See its own
// module docs.
pub mod project_home_row;
// `project_switcher_panel` (P3.60) is unflattened for the same reason.
// `project-home-row.tsx` pushes this panel onto the nav stack on click, so
// it lands alongside its sibling here rather than in
// `super::sidebar::nav_stack` — see `native/mapping/layout-denominator.md`
// §8's Cluster 5. Its own row ids are index-parameterized, not fixed
// strings; see its module docs for why.
pub mod project_switcher_panel;

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
