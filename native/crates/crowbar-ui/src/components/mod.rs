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

// The two gate surfaces are public **modules** as well as flattened exports.
// They each carry an `ID_ITEM`, an `ID_NAME`, a `CONTENT_SIZED` and a
// `guide_id`, and those cannot all be flattened into one namespace — nor should
// they be: `git_status_row::ID_ITEM` and `file_tree_row::ID_ITEM` are different
// anchors on different surfaces, and code that names one should have to say
// which.
pub mod file_tree_row;
pub mod git_status_row;

pub use anchor::{AnchorId, AnchorSink, Unanchored};
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
