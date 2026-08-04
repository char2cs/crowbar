//! `repo_section` — one repo in the workspaces sidebar: its header row (the
//! repo-home default workspace's own row) plus, when expanded, the inline
//! create input, any optimistic pending-create rows, and the workspace tree
//! itself.
//!
//! The native half of `web/src/components/layout/repo-section.tsx`. Third
//! of `native/mapping/layout-denominator.md` §8's Cluster 8 chain: composes
//! [`super::repo_icon_popover::Trigger`], [`super::pending_create_row`] and
//! [`super::workspace_tree_item`]. Every length below came out of the app's
//! own `tailwindcss` 4.3.0 with the utility as a candidate — the method
//! `native/MAPPING.md` fixes. See `native/mapping/repo-section.md`.
//!
//! # `RepoImportDialog` is not composed
//!
//! `repo-section.tsx` renders `<RepoImportDialog ... open={importOpen} .../>`
//! unconditionally, starting closed — the same `AddRepositoryModal`/
//! `ImportProjectModal` posture [`super::project_home_row`]'s and
//! [`super::project_switcher_panel`]'s own module docs already take. This
//! section's own painted picture never depends on whether the dialog is
//! open, so nothing is omitted by leaving it out.
//!
//! # Composes `repo_icon_popover::Trigger::render` directly
//!
//! Single instance per section (there is exactly one avatar per repo row),
//! so this is collision-free the same way [`super::project_home_row`]'s
//! single composition of [`super::workspace_branch_icon::WorkspaceBranchIcon::render`]
//! is. Reusing it rather than hand-rolling a second copy of the avatar's own
//! three-way picture is `repo_icon_popover.rs`'s own module docs'
//! restraint, applied here.
//!
//! # Two foreign, not-yet-ported children, same treatment as
//! `workspace_tree_item.rs`
//!
//! The rename slot (`workspace-inline-input.tsx`, no wrapper of its own) is
//! left unanchored; the root-level create-input row's own chrome (this
//! component's own markup, `WorkspaceInlineInput` sitting inside it) is
//! anchored ([`ID_CREATE_INPUT`]). See `workspace_tree_item.rs`'s module
//! docs for the full argument, restated once rather than twice.
//!
//! # The composed workspace-tree-item family is out of THIS surface's own
//! declared oracle scope
//!
//! `web/src/lib/oracle/extract.ts`'s own `repo-section` entry excludes the
//! recursive `workspace-tree-item`/`pending-create-row` family — their
//! count is a property of the repo's own workspace list (a cell property),
//! not of this surface — `select-item`'s reasoning, one level up. This
//! file still composes and renders them for real (`Self::roots`,
//! `Self::pending_creates`); the exclusion is about what a live oracle
//! capture of `--surface repo-section` compares, not about what this port
//! paints.
//!
//! # `mx-1.5`/`my-0.5` are modelled, and reached via stretch — not `.w_full()`
//!
//! This section's own header row sits beside sibling repo sections under
//! `workspace-tree.tsx`'s own repo list, so [`row_base::MARGIN_X`]/
//! [`row_base::MARGIN_Y`] are applied to it, wrapped in its own `flex
//! flex-col` context — `workspace_tree_item.rs`'s own documented fix for
//! `project_switcher_panel.rs`'s `.w_full()` bug, applied a third time.
//!
//! # The state axis
//!
//! | flag | here |
//! |---|---|
//! | `selected` | **real.** `activeWorkspaceId === repo.defaultWorkspaceId` selects [`row_base::active`] over [`row_base::inactive`], the same reading every other row-shaped surface in this port gives it. |
//! | `empty`, `loading`, `error`, `hover`, `focus` | **unmodelled.** `empty` has no trailing-edge concept on this row. `hover`/`focus` are colour-only ([`row_base`]'s own module docs). `isRepoDragOver` (`ring-1 ring-ring`) is sourced from `workspace-tree-context.tsx`'s Phase 5 drag protocol — out of this item's scope, `workspace_tree_item.rs`'s own dragging/moving/drop-target reasoning, restated once for this surface. |

use gpui::{
    AnyElement, FontWeight, IntoElement as _, ParentElement as _, Pixels, SharedString,
    Styled as _, div, px, relative,
};

use crate::anchor::{AnchorId, AnchorSink};
use crate::surfaces::rows::pending_create_row::PendingCreateRow;
use super::repo_icon_popover::Trigger;
use crate::surfaces::rows::row_base;
use crate::surfaces::workspace::workspace_tree_item::WorkspaceTreeItem;
use crate::theme::{Color, Theme};

/// The section's own anchor. See the module docs for why it is `.boxed`,
/// not `.root` — always composed, by `workspace-tree.tsx`.
pub const ID_ROOT: &str = "repo-section";
/// The truncating repo-name label. Renaming branch only (the rename slot
/// carries no anchor — see the module docs).
pub const ID_LABEL: &str = "repo-section-label";
/// The "Import branches" trailing action.
pub const ID_IMPORT: &str = "repo-section-import";
/// The add-child trailing action. `has_default_workspace` only.
pub const ID_ADD_CHILD: &str = "repo-section-add-child";
/// The collapse/expand trailing action.
pub const ID_COLLAPSE: &str = "repo-section-collapse";
/// The root-level inline create row's own chrome. `is_creating_child` only.
pub const ID_CREATE_INPUT: &str = "repo-section-create-input";

/// [`ID_LABEL`] only — its box equals its own text run's max-content width
/// exactly (see [`RepoSection::label`]'s own doc comment for the two-level
/// structure that makes this true), the `repo_icon_popover.rs` v1.5
/// precedent applied here. Every other box on this surface is authored or
/// `flex-1`.
pub const CONTENT_SIZED: [&str; 1] = [ID_LABEL];
/// [`ID_LABEL`] only — a blockified flex item with no explicit height of
/// its own, [`super::project_home_row::LINE_SIZED`]'s own shape.
pub const LINE_SIZED: [&str; 1] = [ID_LABEL];

/// `14` — the root-level indentation the create row and every direct
/// pending-create row render at (`paddingLeft: 14`, the `depth`-less shape
/// [`super::workspace_tree_item::INDENT`] is one term of).
pub const ROOT_PADDING_LEFT: Pixels = px(14.0);

/// One `<RepoSection>`.
#[derive(Clone, Debug, PartialEq)]
pub struct RepoSection {
    /// `repo.name`.
    pub name: SharedString,
    /// `activeWorkspaceId !== '' && activeWorkspaceId === repo.defaultWorkspaceId`.
    pub is_active: bool,
    /// `isCollapsed`.
    pub is_collapsed: bool,
    /// `renamingRepoId === repo.id`/`creatingChildOf?.repoId === repo.id &&
    /// creatingChildOf?.parentId === repo.defaultWorkspaceId` (already
    /// resolved by the caller), folded into one three-way state — see
    /// [`row_base::RowMode`]'s own doc comment.
    pub mode: row_base::RowMode,
    /// `!!repo.defaultWorkspaceId` — gates the add-child action and the
    /// root-level create row.
    pub has_default_workspace: bool,
    /// The leading avatar/icon-popover trigger.
    pub trigger: Trigger,
    /// `roots.map(...)` — this repo's own top-level workspace rows,
    /// `depth: 0`.
    pub roots: Vec<WorkspaceTreeItem>,
    /// The in-flight creates whose `parentId` is this repo's default
    /// workspace, already filtered and ordered by the caller.
    pub pending_creates: Vec<PendingCreateRow>,
}

impl RepoSection {
    /// A representative cell: one repo, one leaf root workspace, expanded,
    /// inactive, unlocked, no in-flight creates — the live, reachable
    /// resting shape.
    #[must_use]
    pub fn fixture(theme: &Theme) -> Self {
        Self {
            name: SharedString::new_static("crowbar"),
            is_active: false,
            is_collapsed: false,
            mode: row_base::RowMode::Normal,
            has_default_workspace: true,
            trigger: Trigger::fixture(theme),
            roots: vec![WorkspaceTreeItem::fixture()],
            pending_creates: Vec::new(),
        }
    }

    /// The label slot: the repo name, or — while renaming — an empty,
    /// unanchored placeholder standing in for `WorkspaceInlineInput`.
    ///
    /// # Two nested boxes, not [`row_base::label_container`]'s one
    ///
    /// `repo-section.tsx` wraps the name in an *outer*, unanchored `flex
    /// min-w-0 flex-1 items-baseline gap-1.5` span (room for a hover-only
    /// "- default" hint beside it, itself unmodelled — `row_base`'s own
    /// hover restraint applied one level up). The **anchored** name span
    /// nested inside it carries only `min-w-0 truncate font-mono
    /// text-foreground` — no `flex-1` of its own:
    /// `<span className="min-w-0 truncate font-mono text-foreground"
    /// data-oracle-id="repo-section-label">`.
    ///
    /// This file used to reuse [`row_base::label_container`] (`min-w-0
    /// flex-1 truncate`, `flex-1` included) directly for the *anchored* box,
    /// on the reasoning that — with nothing else rendered beside the label —
    /// the wrapper's own box and the label's own box coincide. **That
    /// reasoning was wrong.** Taffy stretches a block-level in-flow child to
    /// fill its own *immediate* container's width regardless of what else
    /// that container holds (`git_status_row.rs`'s own `name_and_directory`
    /// finding: "a flex item is blockified and its text is inline content;
    /// taffy stretches a block-level in-flow child to the container's inner
    /// width"), so a text run placed inside a `flex-1` box reports *that
    /// box's* stretched width, not its own content width, whether or not a
    /// sibling is competing for the leftover space. Measured:
    /// `repo-section-label.bounds.w: 202.0` against a reference `31.2` —
    /// exactly that anchor's own `text_width`, i.e. content-sized
    /// (`native/mapping/repo-section.md`'s own verdict, defect 2).
    ///
    /// The fix restores the two-level structure the source actually has: an
    /// *outer*, unanchored `flex-1` spacer — so the trailing action buttons
    /// still land at the row's own trailing edge exactly as they do today;
    /// dropping the spacer outright would pull them in against a short
    /// label instead — wrapping an *inner*, content-sized box that carries
    /// the anchor. The inner box is a flex item of the outer
    /// (`display:flex`) container with no `flex-grow` of its own (gpui's
    /// own default, `Style::default()`'s `flex_grow: 0.0`), so it sizes to
    /// its own content rather than the row's leftover space, while still
    /// shrinking and truncating if the name is too long for what the
    /// spacer has left — `min-w-0 truncate` on both sides.
    ///
    /// **`project-home-row` and `workspace-tree-item` are not this shape**
    /// — do not "fix" this by making every row label content-sized. Both
    /// put `flex-1` directly on the *same* span that carries their own
    /// label anchor (`project-home-row.tsx`'s label;
    /// `workspace-tree-item.tsx`'s `className="min-w-0 flex-1 truncate
    /// font-mono text-left" data-oracle-id="workspace-tree-item-label"`),
    /// so [`row_base::label_container`] is exactly right for both — a
    /// `project-home-row-label` legitimately at 232px against a 109.2px
    /// `text_width`, a `workspace-tree-item-label` legitimately at 252px
    /// against 31.2px — and neither is touched by this fix. Only this
    /// file's own call site was reading a *wrapper's* class list onto the
    /// wrong element.
    ///
    /// # `content_sized` is declared, not merely true
    ///
    /// The two-level structure above makes `bounds.w` equal `text_width`
    /// exactly, but GPUI still ceils a text run's own max-content width
    /// (`ANCHORS.md` v1.5), so a fractional reference (`31.2`) compares
    /// against this port's own integral `32.0` — a real, if tiny, gap that
    /// the differ can only forgive once the anchor is declared
    /// `content_sized` on **both** sides, `repo_icon_popover.rs`'s own P3.63
    /// precedent. Declared here via `AnchorId::content_sized()`, and on the
    /// React side via `data-oracle-content-sized="true"`
    /// (`repo-section.tsx`). [`super::project_home_row`]'s and
    /// `workspace_tree_item.rs`'s own labels stay undeclared — the
    /// paragraph above is why.
    fn label(&self, theme: &Theme, anchors: &dyn AnchorSink) -> AnyElement {
        // The outer, unanchored spacer: `<span className="flex min-w-0
        // flex-1 items-baseline gap-1.5">`. `items_baseline` rather than
        // `items_center`: the source's own class says baseline, and for a
        // single-line label the two render identically, but they are free
        // to diverge if a second child (the "- default" hint) is ever
        // modelled here.
        let outer = div().flex().min_w(px(0.0)).flex_1().items_baseline();

        if self.mode.is_renaming() {
            return outer.into_any_element();
        }

        // The inner, anchored, content-sized name span: `<span
        // className="min-w-0 truncate font-mono text-foreground"
        // data-oracle-id="repo-section-label">`. Same type as
        // `row_base::label_container` (`row_base::TEXT` /
        // `LINE_HEIGHT_RELATIVE` / `FontWeight::MEDIUM`) but no
        // `.flex_1()` — see this function's own doc comment.
        let inner = div()
            .min_w(px(0.0))
            .truncate()
            .text_size(row_base::TEXT)
            .line_height(relative(row_base::LINE_HEIGHT_RELATIVE))
            .font_weight(FontWeight::MEDIUM)
            .text_color(theme.foreground)
            .font_family(theme.font_mono.primary().unwrap_or("monospace"));

        outer
            .child(inner.child(anchors.text(
                AnchorId::new(ID_LABEL).line_sized().content_sized(),
                self.name.clone(),
            )))
            .into_any_element()
    }

    /// The header row: trigger, label, and up to three trailing actions.
    fn header(&self, theme: &Theme, anchors: &dyn AnchorSink) -> AnyElement {
        // No `.w_full()` — see the module docs.
        let mut shell = if self.is_active {
            row_base::active(theme)
        } else {
            row_base::inactive(theme)
        }
        .mx(row_base::MARGIN_X)
        .my(row_base::MARGIN_Y)
        .child(self.trigger.render(theme, anchors))
        .child(self.label(theme, anchors))
        .child(anchors.boxed(
            AnchorId::from(ID_IMPORT),
            row_base::sub_action_box(theme).child(row_base::sub_action_glyph()),
        ));

        if self.has_default_workspace {
            shell = shell.child(anchors.boxed(
                AnchorId::from(ID_ADD_CHILD),
                row_base::sub_action_box(theme).child(row_base::sub_action_glyph()),
            ));
        }

        shell = shell.child(anchors.boxed(
            AnchorId::from(ID_COLLAPSE),
            row_base::sub_action_box(theme).child(row_base::sub_action_glyph()),
        ));

        div()
            .flex()
            .flex_col()
            .child(anchors.boxed(AnchorId::from(ID_ROOT), shell))
            .into_any_element()
    }

    /// The root-level inline create row. `None` unless
    /// `self.mode.is_creating_child()`.
    fn create_row(&self, theme: &Theme, anchors: &dyn AnchorSink) -> Option<AnyElement> {
        if !self.mode.is_creating_child() {
            return None;
        }
        Some(
            div()
                .flex()
                .flex_col()
                .pl(ROOT_PADDING_LEFT)
                .child(
                    anchors.boxed(
                        AnchorId::from(ID_CREATE_INPUT),
                        row_base::base(theme)
                            .border_color(Color::TRANSPARENT)
                            .text_color(theme.foreground)
                            .child(row_base::sub_action_glyph()),
                    ),
                )
                .into_any_element(),
        )
    }

    /// Renders the section, opting every contract anchor into `anchors`.
    #[must_use]
    pub fn render(&self, theme: &Theme, anchors: &dyn AnchorSink) -> AnyElement {
        let mut outer = div().mb(px(4.0)).child(self.header(theme, anchors));

        if !self.is_collapsed {
            let mut group = div();
            if let Some(create_row) = self.create_row(theme, anchors) {
                group = group.child(create_row);
            }
            for pending in &self.pending_creates {
                group = group.child(pending.render(theme, anchors));
            }
            for root in &self.roots {
                group = group.child(root.render(theme, anchors));
            }
            outer = outer.child(group);
        }

        outer.into_any_element()
    }
}

#[cfg(test)]
mod tests {
    use super::{
        CONTENT_SIZED, ID_ADD_CHILD, ID_COLLAPSE, ID_CREATE_INPUT, ID_IMPORT, ID_LABEL, ID_ROOT,
        LINE_SIZED, ROOT_PADDING_LEFT, RepoSection, row_base,
    };
    use crate::theme::Theme;
    use gpui::px;

    #[test]
    fn every_length_is_the_compiled_value() {
        assert_eq!(ROOT_PADDING_LEFT, px(14.0));
    }

    #[test]
    fn the_declaration_lists_match_what_render_actually_declares() {
        assert_eq!(CONTENT_SIZED, [ID_LABEL]);
        assert_eq!(LINE_SIZED, [ID_LABEL]);
    }

    #[test]
    fn the_six_anchor_ids_are_distinct_and_namespaced() {
        let ids = [
            ID_ROOT,
            ID_LABEL,
            ID_IMPORT,
            ID_ADD_CHILD,
            ID_COLLAPSE,
            ID_CREATE_INPUT,
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
    fn the_fixture_is_one_expanded_repo_with_one_leaf_root() {
        let theme = Theme::DARK;
        let section = RepoSection::fixture(&theme);
        assert!(!section.is_active);
        assert!(!section.is_collapsed);
        assert_eq!(section.mode, row_base::RowMode::Normal);
        assert!(section.has_default_workspace);
        assert_eq!(section.roots.len(), 1);
        assert!(section.pending_creates.is_empty());
    }
}
