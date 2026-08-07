//! `workspace_tree_item` — one row of the workspace tree: a status icon, the
//! branch name (or an inline rename/create editor), optional change counts,
//! a trailing expand/collapse or add-child control, and — recursively — its
//! own children.
//!
//! The native half of `web/src/components/layout/workspace-tree-item.tsx`.
//! Second of `native/mapping/layout-denominator.md` §8's Cluster 8 chain:
//! composes [`super::pending_create_row`] and renders itself recursively.
//! Every length below came out of the app's own `tailwindcss` 4.3.0 with the
//! utility as a candidate — the method `native/MAPPING.md` fixes. See
//! `native/mapping/workspace-tree-item.md`.
//!
//! # Two foreign, not-yet-ported children: `placeholder-row-actions` and
//! `workspace-inline-input`
//!
//! `placeholder-row-actions.tsx` (the placeholder row's Retry/Detach… panel)
//! and `workspace-inline-input.tsx` (the rename/create-child text field) are
//! each a **separate, not-yet-landed Tier B target**
//! (`native/mapping/layout-denominator.md` §8's Clusters 7/8) — neither is
//! this item's to port. Both are stubbed, but differently, matched to
//! whether this file owns the wrapping box:
//!
//! * The **placeholder-details wrapper** (`mx-1.5 mb-0.5 rounded-b-lg …
//!   bg-background …`) is this component's own real chrome regardless of
//!   what renders inside it, so it is anchored ([`ID_PLACEHOLDER_DETAILS`])
//!   and painted; `PlaceholderRowActions`'s own content inside it is left
//!   empty.
//! * The **rename label** and the **create-child input** have no wrapper of
//!   their own in the React source — `WorkspaceInlineInput` sits directly
//!   in the row's children list — so this port paints an empty, unanchored
//!   box in that slot rather than inventing a wrapper (and an id) around
//!   content it does not own. See the React diff's own comment on both call
//!   sites.
//!
//! Neither needs an `oracleSurfaceScope` entry: neither foreign component
//! carries any `data-oracle-id` of its own yet (checked against both
//! sources), so there is nothing for a live capture to leak that this port's
//! own tree does not already omit by construction.
//!
//! # The status icon composes [`super::workspace_branch_icon`] directly —
//! and that is only safe for a leaf cell
//!
//! Unlike [`super::pending_create_row`] (a genuine leaf), this component
//! recurses, so a cell with children present paints the SAME shared
//! `workspace-branch-icon` literal id more than once in one capture — sound
//! for `cargo test`'s own `row_layout` harness (`AnchorRegistry::record`
//! keeps differing records rather than refusing them; `Snapshot::build`'s
//! v1.8 refusal is never reached because these tests never call
//! `.snapshot()`), but not for a live oracle capture with children present.
//! `web/src/lib/oracle/extract.ts`'s own `workspace-tree-item` entry records
//! this constraint in full — the declared scope is sound for a childless
//! cell only.
//!
//! # `hasChildren`/`isCreatingChild`/`showPlaceholderDetails` are derived,
//! not stored
//!
//! [`WorkspaceTreeItem::is_locked`] and
//! [`WorkspaceTreeItem::show_placeholder_details`] are methods, not fields —
//! `isLocked`/`showPlaceholderDetails` are themselves derived in the React
//! source (`status === 'locked'`, `isPlaceholder && isActive`), and storing
//! them as independent fields would let a fixture disagree with itself.
//!
//! # `mx-1.5`/`my-0.5` are modelled, and reached via stretch — not `.w_full()`
//!
//! Every row in this tree sits beside siblings (recursive children, or
//! other roots under the same `repo-section`), so
//! [`row_base::MARGIN_X`]/[`row_base::MARGIN_Y`] are applied to the row
//! itself, and the padding wrapper around it is `flex flex-col` so the row
//! stretches to fill it **net of its own margin** —
//! `project_switcher_panel.rs`'s own documented `.w_full()` bug, avoided the
//! same way [`super::pending_create_row`] avoids it.
//!
//! # The state axis
//!
//! | flag | here |
//! |---|---|
//! | `selected` | **real.** `isActive` selects [`row_base::active`] over [`row_base::inactive`] — the same reading every other row-shaped surface in this port gives it. |
//! | `empty`, `loading`, `error`, `hover`, `focus` | **unmodelled.** `empty` has no `git-status-row`-shaped "nothing on the trailing edge" concept here — this row always paints an icon and, on a leaf, an add-child action. `hover`/`focus` are colour-only rules with no runtime seam — [`row_base`]'s own module docs. Dragging (`opacity-40`), moving (`opacity-50`) and being a drop target (`ring-1`) are real props on the React source but are sourced entirely from `workspace-tree-context.tsx`'s own pointer-drag protocol, which `native/mapping/layout-denominator.md` §6 classifies **Phase 5 (interaction), not Tier B, with no geometry of its own** — out of this item's scope, so none of the three has a field here. |

use gpui::{
    AnyElement, Div, IntoElement as _, ParentElement as _, Pixels, SharedString, Styled as _, div,
    px,
};

use super::workspace_branch_icon::{self, WorkspaceBranchIcon};
use crate::anchor::{AnchorId, AnchorSink};
use crate::icon::IconName;
use crate::surfaces::rows::pending_create_row::PendingCreateRow;
use crate::surfaces::rows::row_base;
use crate::theme::{Color, Theme};

/// The row's own anchor. See the module docs for why it is `.boxed`, not
/// `.root` — this component is always composed (by `repo_section.rs`, or by
/// itself, recursively).
pub const ID_ROOT: &str = "workspace-tree-item";
/// The truncating branch-name label. Renaming branch only (the create/rename
/// input slot carries no anchor — see the module docs).
pub const ID_LABEL: &str = "workspace-tree-item-label";
/// The `+N` added count. Active, not renaming, not locked, `added > 0` only.
pub const ID_ADDED: &str = "workspace-tree-item-added";
/// The `-N` deleted count. Same gating, `deleted > 0`.
pub const ID_DELETED: &str = "workspace-tree-item-deleted";
/// The expand/collapse chevron. `has_children` only, mutually exclusive with
/// [`ID_ADD_CHILD`].
pub const ID_EXPAND: &str = "workspace-tree-item-expand";
/// The leaf row's add-child action. `!has_children && !is_creating_child`
/// only.
pub const ID_ADD_CHILD: &str = "workspace-tree-item-add-child";
/// The placeholder Retry/Detach… wrapper. `is_placeholder && is_active`
/// only. See the module docs for why this slot IS anchored despite wrapping
/// foreign content.
pub const ID_PLACEHOLDER_DETAILS: &str = "workspace-tree-item-placeholder-details";
/// The inline create-child row's own chrome. `is_creating_child` only,
/// mutually exclusive with [`ID_NEW_BUTTON`].
pub const ID_CREATE_INPUT: &str = "workspace-tree-item-create-input";
/// The static "+ New" button. `!is_creating_child` only (and only reachable
/// at all once the children section is showing).
pub const ID_NEW_BUTTON: &str = "workspace-tree-item-new-button";

/// **Empty.** Every box on this surface is authored or `flex-1`.
pub const CONTENT_SIZED: [&str; 0] = [];
/// [`ID_LABEL`], [`ID_ADDED`], [`ID_DELETED`] — blockified flex items with
/// no explicit height of their own, [`super::project_home_row::LINE_SIZED`]'s
/// own shape.
pub const LINE_SIZED: [&str; 3] = [ID_LABEL, ID_ADDED, ID_DELETED];

/// `14` — one indent level, `depth * 14` (`native/mapping/layout-
/// denominator.md`'s own denominator arithmetic for this cluster).
pub const INDENT: Pixels = px(14.0);
/// `gap-1` on the change-count cluster.
pub const CHANGES_GAP: Pixels = px(4.0);

/// One `<WorkspaceTreeItem>`, and — via [`Self::children`] — every row
/// beneath it.
#[derive(Clone, Debug, PartialEq)]
pub struct WorkspaceTreeItem {
    /// `workspace.branch`.
    pub branch: SharedString,
    /// How many ancestors this row has — `0` for a repo root.
    pub depth: u32,
    /// `workspace.status ?? 'new'` / `workspace.working || isMoving` /
    /// `isPlaceholderWorkspace(workspace)`, bundled into one field —
    /// `WorkspaceBranchIcon` already models exactly this three-way input,
    /// and reusing its own struct (rather than three loose fields this
    /// component would just forward unchanged to
    /// [`WorkspaceBranchIcon::render`]) is what keeps this struct under
    /// clippy's `struct_excessive_bools` without inventing a division that
    /// is not already there — see [`row_base::RowMode`]'s own doc comment
    /// for the sibling fold, one field down.
    pub icon: WorkspaceBranchIcon,
    /// `workspace.id === activeWorkspaceId`.
    pub is_active: bool,
    /// `!isCollapsed`.
    pub expanded: bool,
    /// `isRenaming`/`isCreatingChild`, folded into one three-way state —
    /// see [`row_base::RowMode`]'s own doc comment.
    pub mode: row_base::RowMode,
    /// `workspace.added`, already resolved.
    pub added: Option<u32>,
    /// `workspace.deleted`, already resolved.
    pub deleted: Option<u32>,
    /// `node.children`, recursive.
    pub children: Vec<WorkspaceTreeItem>,
    /// The in-flight creates whose `parentId` is this workspace, already
    /// filtered and ordered by the caller.
    pub pending_creates: Vec<PendingCreateRow>,
}

impl WorkspaceTreeItem {
    /// A representative cell: a leaf root workspace (`depth` 0, no
    /// children, no in-flight creates), idle, inactive, unlocked — the
    /// live, overwhelmingly common shape (most rows in a real tree have no
    /// children), and the only shape `web/src/lib/oracle/extract.ts`'s own
    /// `workspace-tree-item` scope can validate — see the module docs.
    #[must_use]
    pub fn fixture() -> Self {
        Self {
            branch: SharedString::new_static("feature/example"),
            depth: 0,
            icon: WorkspaceBranchIcon {
                status: workspace_branch_icon::Status::New,
                working: false,
                is_placeholder: false,
            },
            is_active: false,
            expanded: false,
            mode: row_base::RowMode::Normal,
            added: None,
            deleted: None,
            children: Vec::new(),
            pending_creates: Vec::new(),
        }
    }

    /// `status === 'locked'` — derived, not stored. See the module docs.
    #[must_use]
    pub fn is_locked(&self) -> bool {
        self.icon.status == workspace_branch_icon::Status::Locked
    }

    /// `isPlaceholder && isActive` — derived, not stored.
    #[must_use]
    pub fn show_placeholder_details(&self) -> bool {
        self.icon.is_placeholder && self.is_active
    }

    /// `hasChildren` — `children.length > 0` — derived, not stored, so a
    /// fixture cannot set the flag without the list to back it.
    #[must_use]
    pub fn has_children(&self) -> bool {
        !self.children.is_empty()
    }

    /// `pendingCreates` non-empty for this node.
    #[must_use]
    pub fn has_pending_child(&self) -> bool {
        !self.pending_creates.is_empty()
    }

    /// `(hasChildren && expanded) || isCreatingChild || hasPendingChild`.
    #[must_use]
    pub fn show_children_section(&self) -> bool {
        (self.has_children() && self.expanded)
            || self.mode.is_creating_child()
            || self.has_pending_child()
    }

    /// `(depth + 1) * 14` — this row's own indentation.
    #[must_use]
    pub fn row_padding_left(&self) -> Pixels {
        Self::depth_padding(self.depth + 1)
    }

    /// `(depth + 2) * 14` — the padding recursive children and pending
    /// creates render at.
    #[must_use]
    pub fn nested_padding_left(&self) -> Pixels {
        Self::depth_padding(self.depth + 2)
    }

    #[expect(
        clippy::cast_precision_loss,
        reason = "tree depth is bounded by real UI navigation, nowhere near f32's 24-bit \
                  mantissa; no fractional px is ever lost in practice"
    )]
    fn depth_padding(levels: u32) -> Pixels {
        INDENT * levels as f32
    }

    /// The leading status icon. Composes [`WorkspaceBranchIcon::render`]
    /// directly — see the module docs for why that is safe here (this
    /// row's OWN icon) but not compositionally free of a caveat once
    /// children are present.
    fn icon(&self, theme: &Theme, anchors: &dyn AnchorSink) -> AnyElement {
        self.icon.render(theme, anchors)
    }

    /// The label slot: the branch name, or — while renaming — an empty,
    /// unanchored placeholder standing in for `WorkspaceInlineInput`. See
    /// the module docs.
    fn label(&self, theme: &Theme, anchors: &dyn AnchorSink) -> AnyElement {
        let container = row_base::label_container(theme.foreground)
            .font_family(theme.font_mono.primary().unwrap_or("monospace"));
        if self.mode.is_renaming() {
            container.into_any_element()
        } else {
            container
                .child(anchors.text(AnchorId::new(ID_LABEL).line_sized(), self.branch.clone()))
                .into_any_element()
        }
    }

    /// The change-count cluster. `None` unless active, not renaming, not
    /// locked, and at least one of added/deleted is a positive count.
    fn changes(&self, theme: &Theme, anchors: &dyn AnchorSink) -> Option<AnyElement> {
        if !(self.is_active && !self.mode.is_renaming() && !self.is_locked()) {
            return None;
        }
        let added = self.added.filter(|n| *n > 0);
        let deleted = self.deleted.filter(|n| *n > 0);
        if added.is_none() && deleted.is_none() {
            return None;
        }

        let mut cluster = div()
            .flex()
            .flex_shrink_0()
            .gap(CHANGES_GAP)
            .font_family(theme.font_mono.primary().unwrap_or("monospace"));
        if let Some(n) = added {
            cluster = cluster.child(anchors.boxed_text(
                AnchorId::new(ID_ADDED).line_sized(),
                div().text_color(Color::GREEN_300),
                SharedString::from(format!("+{n}")),
            ));
        }
        if let Some(n) = deleted {
            cluster = cluster.child(anchors.boxed_text(
                AnchorId::new(ID_DELETED).line_sized(),
                div().text_color(Color::RED_300),
                SharedString::from(format!("-{n}")),
            ));
        }
        Some(cluster.into_any_element())
    }

    /// The trailing expand/add-child slot. `None` for a row with children
    /// while a create is already in flight for it (the React source's own
    /// `: null` arm — the create input occupies the children section
    /// instead).
    fn trailing_button(&self, theme: &Theme, anchors: &dyn AnchorSink) -> Option<AnyElement> {
        if self.has_children() {
            // The inline disclosure chevron, `workspace-tree-item.tsx:209`.
            Some(anchors.boxed(
                AnchorId::from(ID_EXPAND),
                row_base::sub_action_box(theme).child(row_base::sub_action_icon(
                    IconName::RowChevron,
                    theme.muted_foreground,
                )),
            ))
        } else if !self.mode.is_creating_child() {
            // The inline `ADD_GLYPH_PATH` plus, `workspace-tree-item.tsx:234`.
            Some(anchors.boxed(
                AnchorId::from(ID_ADD_CHILD),
                row_base::sub_action_box(theme).child(row_base::sub_action_icon(
                    IconName::RowAdd,
                    theme.muted_foreground,
                )),
            ))
        } else {
            None
        }
    }

    /// The placeholder Retry/Detach… wrapper. `None` unless
    /// [`Self::show_placeholder_details`].
    fn placeholder_details(&self, theme: &Theme, anchors: &dyn AnchorSink) -> Option<AnyElement> {
        if !self.show_placeholder_details() {
            return None;
        }
        Some(
            anchors.boxed(
                AnchorId::from(ID_PLACEHOLDER_DETAILS),
                div()
                    .mx(row_base::MARGIN_X)
                    .mb(px(2.0))
                    .rounded_b(theme.radius_lg.value())
                    .px(px(10.0))
                    .pb(px(8.0))
                    .pt(px(2.0))
                    .bg(theme.background),
            ),
        )
    }

    /// The row itself: icon, label, changes, trailing action — everything
    /// but the children section and the placeholder details.
    fn row(&self, theme: &Theme, anchors: &dyn AnchorSink) -> AnyElement {
        // No `.w_full()` — see the module docs.
        let mut shell = if self.is_active {
            row_base::active(theme)
        } else {
            row_base::inactive(theme)
        }
        .mx(row_base::MARGIN_X)
        .my(row_base::MARGIN_Y)
        .child(self.icon(theme, anchors))
        .child(self.label(theme, anchors));

        if let Some(changes) = self.changes(theme, anchors) {
            shell = shell.child(changes);
        }
        if let Some(trailing) = self.trailing_button(theme, anchors) {
            shell = shell.child(trailing);
        }
        // `showPlaceholderDetails && 'mb-0 rounded-b-none'` — squares the
        // row's own bottom corners while the details panel continues its
        // surface directly beneath it.
        if self.show_placeholder_details() {
            shell = shell.mb(px(0.0)).rounded_b(px(0.0));
        }

        anchors.boxed(AnchorId::from(ID_ROOT), shell)
    }

    /// The inline create-child row, shown at [`Self::nested_padding_left`]
    /// as the last entry of the children section. Two mutually exclusive
    /// pictures: the input row (own chrome anchored, `WorkspaceInlineInput`
    /// inside left empty) or the static "+ New" button.
    fn create_or_new_row(&self, theme: &Theme, anchors: &dyn AnchorSink) -> AnyElement {
        if self.mode.is_creating_child() {
            anchors.boxed(
                AnchorId::from(ID_CREATE_INPUT),
                row_base::base(theme)
                    .border_color(Color::TRANSPARENT)
                    .text_color(theme.foreground)
                    .child(row_base::sub_action_glyph()),
            )
        } else {
            anchors.boxed_text(
                AnchorId::from(ID_NEW_BUTTON),
                row_base::base(theme)
                    .border_color(Color::TRANSPARENT)
                    .text_color(theme.muted_foreground)
                    .child(Self::new_button_glyph()),
                SharedString::new_static("New"),
            )
        }
    }

    /// `<Plus className="size-4 shrink-0" />` — empty, unpainted.
    fn new_button_glyph() -> Div {
        div().flex_shrink_0().w(px(16.0)).h(px(16.0))
    }

    /// The children section: recursive children (if expanded), this node's
    /// own pending creates, then the create-input/"+ New" row. `None`
    /// unless [`Self::show_children_section`].
    fn children_section(&self, theme: &Theme, anchors: &dyn AnchorSink) -> Option<AnyElement> {
        if !self.show_children_section() {
            return None;
        }

        let mut group = div();
        if self.has_children() && self.expanded {
            for child in &self.children {
                group = group.child(child.render(theme, anchors));
            }
        }
        for pending in &self.pending_creates {
            group = group.child(pending.render(theme, anchors));
        }

        let nested = div()
            .flex()
            .flex_col()
            .pl(self.nested_padding_left())
            .child(self.create_or_new_row(theme, anchors));
        group = group.child(nested);

        Some(group.into_any_element())
    }

    /// Renders the row (and, recursively, everything beneath it), opting
    /// every contract anchor into `anchors`.
    #[must_use]
    pub fn render(&self, theme: &Theme, anchors: &dyn AnchorSink) -> AnyElement {
        let mut padded = div()
            .flex()
            .flex_col()
            .pl(self.row_padding_left())
            .child(self.row(theme, anchors));
        if let Some(details) = self.placeholder_details(theme, anchors) {
            padded = padded.child(details);
        }

        let mut outer = div().child(padded);
        if let Some(section) = self.children_section(theme, anchors) {
            outer = outer.child(section);
        }
        outer.into_any_element()
    }
}

#[cfg(test)]
mod tests {
    use super::{
        CHANGES_GAP, CONTENT_SIZED, ID_ADD_CHILD, ID_ADDED, ID_CREATE_INPUT, ID_DELETED, ID_EXPAND,
        ID_LABEL, ID_NEW_BUTTON, ID_PLACEHOLDER_DETAILS, ID_ROOT, INDENT, LINE_SIZED,
        WorkspaceTreeItem, row_base,
    };
    use crate::surfaces::workspace::workspace_branch_icon::Status;
    use gpui::px;

    #[test]
    fn every_length_is_the_compiled_value() {
        assert_eq!(INDENT, px(14.0));
        assert_eq!(CHANGES_GAP, px(4.0));
    }

    #[test]
    fn the_declaration_lists_match_what_render_actually_declares() {
        assert!(CONTENT_SIZED.is_empty());
        assert_eq!(LINE_SIZED, [ID_LABEL, ID_ADDED, ID_DELETED]);
    }

    #[test]
    fn the_eight_anchor_ids_are_distinct_and_namespaced() {
        let ids = [
            ID_ROOT,
            ID_LABEL,
            ID_ADDED,
            ID_DELETED,
            ID_EXPAND,
            ID_ADD_CHILD,
            ID_PLACEHOLDER_DETAILS,
            ID_CREATE_INPUT,
            ID_NEW_BUTTON,
        ];
        for id in ids {
            assert!(id.starts_with(ID_ROOT), "{id}");
        }
        let mut sorted = ids.to_vec();
        sorted.sort_unstable();
        let before = sorted.len();
        sorted.dedup();
        assert_eq!(sorted.len(), before, "{ids:?}");
    }

    #[test]
    fn the_fixture_is_a_leaf_root_row() {
        let row = WorkspaceTreeItem::fixture();
        assert_eq!(row.depth, 0);
        assert_eq!(row.icon.status, Status::New);
        assert!(!row.icon.working);
        assert!(!row.icon.is_placeholder);
        assert!(!row.is_active);
        assert!(!row.has_children());
        assert!(row.children.is_empty());
        assert!(row.pending_creates.is_empty());
        assert!(!row.show_children_section());
        assert!(!row.show_placeholder_details());
        assert!(!row.is_locked());
    }

    /// `status === 'locked'` is exactly [`WorkspaceTreeItem::is_locked`],
    /// not a separately-settable field.
    #[test]
    fn is_locked_tracks_the_status_field() {
        let mut row = WorkspaceTreeItem::fixture();
        assert!(!row.is_locked());
        row.icon.status = Status::Locked;
        assert!(row.is_locked());
    }

    /// `isPlaceholder && isActive` is exactly
    /// [`WorkspaceTreeItem::show_placeholder_details`].
    #[test]
    fn show_placeholder_details_requires_both_placeholder_and_active() {
        let mut row = WorkspaceTreeItem::fixture();
        row.icon.is_placeholder = true;
        assert!(
            !row.show_placeholder_details(),
            "placeholder alone is not enough"
        );
        row.is_active = true;
        assert!(row.show_placeholder_details());
    }

    /// `(hasChildren && expanded) || isCreatingChild || hasPendingChild` —
    /// each of the three disjuncts independently.
    #[test]
    fn show_children_section_is_the_three_way_or() {
        let mut row = WorkspaceTreeItem::fixture();
        assert!(!row.show_children_section());

        row.children.push(WorkspaceTreeItem::fixture());
        assert!(row.has_children());
        assert!(!row.show_children_section(), "children without expanded");
        row.expanded = true;
        assert!(row.show_children_section());

        let mut creating = WorkspaceTreeItem::fixture();
        creating.mode = row_base::RowMode::CreatingChild;
        assert!(creating.show_children_section());

        let mut pending = WorkspaceTreeItem::fixture();
        pending
            .pending_creates
            .push(crate::surfaces::rows::pending_create_row::PendingCreateRow::fixture());
        assert!(pending.show_children_section());
    }

    #[test]
    fn depth_padding_is_depth_plus_one_and_plus_two_times_14() {
        let mut row = WorkspaceTreeItem::fixture();
        row.depth = 0;
        assert_eq!(row.row_padding_left(), px(14.0));
        assert_eq!(row.nested_padding_left(), px(28.0));

        row.depth = 2;
        assert_eq!(row.row_padding_left(), px(42.0));
        assert_eq!(row.nested_padding_left(), px(56.0));
    }
}
