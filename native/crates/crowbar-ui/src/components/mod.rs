//! The design system's components.
//!
//! The first of them is the Phase 1 gate: [`GitStatusRow`], one row of the git
//! status panel, built to be measured against the React original across the
//! §8.3 matrix. Everything it paints comes out of [`Theme`](crate::Theme) —
//! there is no colour literal here and the build will not let there be one
//! (`scripts/check-invariants.sh` rule 4).
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
mod git_status_row;
mod sidebar_tree;

pub use anchor::{AnchorSink, Unanchored};
pub use git_status_row::{
    BADGE_LABEL, ContentLength, GitStatusRow, ID_ADDED, ID_BADGE, ID_BUTTON, ID_DELETED, ID_DIR,
    ID_ICON, ID_ITEM, ID_NAME, NAME_MAX_FRACTION, NameSizing, TrailingContent, guide_id,
    name_sizing, shows_directory_span, split_path,
};
pub use sidebar_tree::{
    BASE_INDENT, GUIDE_END_INSET, GUIDE_OPACITY_PERCENT, GUIDE_RULE_INSET, GUIDE_RULE_WIDTH,
    GUIDE_SHIFT, GUIDE_WIDTH, GuideInset, ICON_SIZE, INDENT_SIZE, ITEM_RADIUS, ROW_HEIGHT,
    RowState, guide, guide_inset, guide_left, item, item_background, leading_padding, row_button,
};
