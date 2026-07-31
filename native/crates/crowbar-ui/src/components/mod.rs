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
pub mod dropdown_menu;
pub mod file_tree_row;
pub mod git_status_row;
pub mod resizable;
// `sidebar_carousel` is flattened not at all for the same reason: its
// `CONTENT_SIZED`, `LINE_SIZED` and `ID_*` are its own, and a declaration list
// that silently meant another surface's is the mistake `ANCHORS.md` v1.6 warns
// about.
pub mod sidebar_carousel;
// `tabs` is unflattened for the third time and the same reason: its
// `Orientation`, `Variant`, `Panel`, `Tab`, `CONTENT_SIZED` and `LINE_SIZED`
// would each collide with another surface's, and a declaration list that
// silently meant `resizable`'s is the mistake `ANCHORS.md` v1.6 warns about.
// `Panel` is the sharpest of them: `resizable::Panel` is a resize pane and
// `tabs::Panel` is a tab's content, and nothing about the name says which.
pub mod tabs;

pub use anchor::{AnchorId, AnchorSink, Unanchored};
pub use dropdown_menu::DropdownMenu;
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
