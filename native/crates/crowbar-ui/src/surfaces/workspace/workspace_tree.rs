//! `workspace_tree` — the sidebar's own workspace tree: the project-home
//! row, a scrolling column of repo sections, each holding its own workspace
//! rows.
//!
//! The native half of `web/src/components/layout/workspace-tree.tsx`. Last
//! of `native/mapping/layout-denominator.md` §8's Cluster 8 chain, and the
//! chain's own root: composes [`super::project_home_row`] and
//! [`super::repo_section`]. Every length below came out of the app's own
//! `tailwindcss` 4.3.0 with the utility as a candidate — the method
//! `native/MAPPING.md` fixes. See `native/mapping/workspace-tree.md`.
//!
//! # This is the one component in the chain that IS `anchors.root`
//!
//! [`super::pending_create_row`], [`super::workspace_tree_item`] and
//! [`super::repo_section`] are all always composed (by a parent, or by
//! themselves recursively) and so all use [`AnchorSink::boxed`] for their
//! own root — see each one's own module docs. Nothing composes
//! `WorkspaceTree` (`workspace-tree.tsx`'s sole importer is `sidebar-
//! carousel.tsx`, which — per `native/mapping/layout-denominator.md` §1 —
//! renders the four panels it wraps **empty**, this port included), so this
//! file is the chain's genuine top: [`WorkspaceTree::render`] calls
//! [`AnchorSink::root`], [`super::project_home_row::ProjectHomeRow`]'s own
//! shape.
//!
//! # `WorkspaceTreeFooter` is omitted, not stubbed
//!
//! `workspace-tree.tsx` renders `<WorkspaceTreeFooter />` as a sibling of
//! the scroll area, always mounted ("always rendered so the `ScrollArea`
//! doesn't resize on drag start/end", the source's own comment).
//! `native/mapping/layout-denominator.md` §6 classifies it **Phase 5
//! (interaction — drag), not Tier B**, and flags the resulting composition
//! decision as unmade — this item makes it: **omit**, not stub.
//!
//! Its own resting picture (no drag in progress) is `max-h-0 overflow-
//! hidden` — zero height, by its own class list, regardless of whether the
//! element exists at all. Its data feed (`draggingWs`/`hoverTargetId` off
//! `useWorkspaceTreeDrag()`) is `workspace-tree-context.tsx`'s Phase 5
//! pointer-drag protocol, itself out of Tier B scope and not composed by
//! anything in this chain. So an omitted footer and a present-but-
//! `max-h-0` one paint the identical picture in every state this port's own
//! resting captures reach — the same `AddRepositoryModal`-shaped argument
//! [`super::project_home_row`]'s own module docs make ("the row's own
//! painted picture never depends on whether the dialog is open, so nothing
//! is omitted by leaving it out"), reapplied to a sibling rather than a
//! portal. A worker who later ports `workspace-tree-footer.tsx` for its own
//! sake (Phase 5) is free to compose it here; this item does not block that.
//!
//! # `ScrollArea::render` is not called; its two ids are reused by hand
//!
//! [`super::scroll_area::ScrollArea::render`] paints a **synthetic, empty**
//! body at a caller-fixed pixel extent — its own module docs: "unanchored on
//! purpose... it stands in for whatever a call site rendered." This
//! composition needs the opposite: the real repo-section list, sized by
//! `flex-1` rather than a fixed number. So this file does not call
//! `ScrollArea::render` — the same "does not call `X::render`" posture
//! `project_home_row.rs`/`context_pill.rs` already take for `Button::
//! render`, generalized here to a primitive whose own API has no children
//! slot at all — and instead hand-builds the equivalent
//! `overflow-hidden` viewport shape, reusing [`super::scroll_area::ID_ROOT`]/
//! [`super::scroll_area::ID_VIEWPORT`] directly: the real `<ScrollArea>` in
//! `workspace-tree.tsx` carries exactly those two ids on its root and
//! viewport regardless of what it wraps, so reusing the constants keeps
//! this port's anchor set matching the real DOM rather than inventing a
//! third name for the same two boxes.
//!
//! # The composed repo-section family is out of THIS surface's own declared
//! oracle scope
//!
//! `web/src/lib/oracle/extract.ts`'s own `workspace-tree` entry excludes
//! `repo-section` — `resizable`'s own precedent for a container whose
//! repeated content is verified through that content's own surface, not
//! through the container's. This file still composes and renders every
//! section for real; the exclusion is about what a live oracle capture of
//! `--surface workspace-tree` compares.
//!
//! # `project_home` is composed beside `repo_section`'s own list — so it
//! takes `row_base::MARGIN_X`/`MARGIN_Y` too
//!
//! [`row_base`]'s own module docs name this file specifically: `mx-1.5`/
//! `my-0.5` are exported "as the numbers a *list*-shaped consumer
//! (`project-switcher-panel`, **eventually `workspace-tree`**) applies to
//! each row itself" — because a row captured on its own has no anchor those
//! two lengths could move, but a row sitting above a scrolling column of
//! siblings does. `ProjectHomeRow::render` composes `row_base::active`/
//! `inactive` with `.w_full()`, not `.mx()`/`.my()` — correct for *its own*
//! standalone `--surface project-home-row` capture
//! (`native/mapping/project-home-row.md`'s own PASS verdict, root `332×36`
//! exactly matching `--width 332`, no inset) — and this file used to call
//! `self.project_home.render(...)` straight into its own column with
//! nothing further, which is the bug: composed here, the row sits beside
//! `repo-section`'s own list exactly the way `project-switcher-panel.rs`'s
//! rows sit beside each other, and the margin never got applied at this,
//! the only call site that needed it.
//!
//! [`Self::home_row`] applies it **without touching `ProjectHomeRow::render`
//! itself** — the "no `.w_full()` next to `.mx()`" trap
//! `workspace_tree_item.rs`'s own module docs name, avoided here by putting
//! the margin on an *outer*, unanchored `flex flex-col` wrapper rather than
//! on the same div that already carries `.w_full()`. The wrapper is the
//! single item of its own `flex-col` context, so it stretches to this
//! column's own width **net of its own margin** (the same stretch-minus-
//! margin arithmetic `repo_section.rs`'s header and `workspace_tree_item.
//! rs`'s row already rely on); `project_home`'s inner `.w_full()` then
//! fills 100% of *that* already-inset width, landing the anchored
//! `project-home-row` box at the right position without either file
//! needing to agree on which one owns the margin. Measured before this fix
//! (`native/mapping/workspace-tree.md`'s own verdict, cause A, 13 of its 19
//! deltas):
//!
//! ```text
//! project-home-row.bounds.x:        0.0,  expected 6.0    (mx-1.5)
//! project-home-row.bounds.y:        0.0,  expected 2.0    (my-0.5)
//! project-home-row.bounds.w:      344.0,  expected 332.0  (344 − 2×6)
//! scroll-area-{root,viewport}.bounds.y:   36.0, expected 40.0
//! workspace-tree.bounds.h:               972.0, expected 976.0
//! ```
//!
//! plus every one of `project-home-row`'s own children carried along by the
//! row's shifted origin. The last two follow from the first two without any
//! further code: `my-0.5` on both edges adds `MARGIN_Y * 2` (4px) to the
//! column's own flow height before the scroll area starts, which is
//! `36 + 4 = 40` and `972 + 4 = 976`.
//!
//! # The `error` early return is not modelled
//!
//! `wsListData.status === 'error' && repos.length === 0` swaps the whole
//! tree for `<InlineError>` — `StateFlag::Error` is mandatorily unmodelled
//! on every surface in this port (`crowbar-app/src/surface.rs`'s own
//! invariant), so this struct has no field for it and the React diff does
//! not tag that branch's own root with `data-oracle-id="workspace-tree"` —
//! see the React diff's own comment.
//!
//! # The state axis
//!
//! | flag | here |
//! |---|---|
//! | `loading`, `error`, `hover`, `focus`, `selected`, `empty` | **unmodelled.** This surface is a container, not a row — it has no selection/hover/focus picture of its own, and its own `empty` (zero repos) would be a real, drivable state but is left for a future worker to add rather than guessed at here: nothing in `native/mapping/layout-denominator.md` flags a live "zero repos" reference, and inventing the field without one to validate against is exactly the trap `ANCHORS.md`'s own §6 warns against. |

use gpui::{AnyElement, IntoElement as _, ParentElement as _, Pixels, Styled as _, div, px};

use crate::anchor::{AnchorId, AnchorSink};
use crate::surfaces::rows::project_home_row::ProjectHomeRow;
use crate::surfaces::repo::repo_section::RepoSection;
use crate::surfaces::rows::row_base;
use crate::primitives::scroll_area;
use crate::theme::Theme;

/// The tree's own anchor. See the module docs for why this is the one
/// component in the chain that calls [`AnchorSink::root`].
pub const ID_ROOT: &str = "workspace-tree";

/// One `<WorkspaceTree>`.
#[derive(Clone, Debug, PartialEq)]
pub struct WorkspaceTree {
    /// `<ProjectHomeRow />`, unconditional.
    pub project_home: ProjectHomeRow,
    /// `repos.map(...)`.
    pub sections: Vec<RepoSection>,
    /// The scroll area's own extent — the call site's `flex-1` column,
    /// measured live at `344 x 936` for the real `workspace-tree` instance
    /// ([`super::scroll_area::ScrollArea::fixture`]'s own reference).
    pub scroll_width: Pixels,
    /// The same, vertically.
    pub scroll_height: Pixels,
}

impl WorkspaceTree {
    /// A representative cell: the project-home row idle/inactive, one repo
    /// with one leaf root workspace, at `ScrollArea::fixture`'s own live
    /// extent.
    #[must_use]
    pub fn fixture(theme: &Theme) -> Self {
        Self {
            project_home: ProjectHomeRow::fixture(),
            sections: vec![RepoSection::fixture(theme)],
            scroll_width: px(344.0),
            scroll_height: px(936.0),
        }
    }

    /// `project_home`, inset by `row_base::MARGIN_X`/`MARGIN_Y` — see the
    /// module docs for why the inset lives on this outer wrapper rather
    /// than inside `ProjectHomeRow::render` itself.
    fn home_row(&self, theme: &Theme, anchors: &dyn AnchorSink) -> AnyElement {
        div()
            .flex()
            .flex_col()
            .mx(row_base::MARGIN_X)
            .my(row_base::MARGIN_Y)
            .child(self.project_home.render(theme, anchors))
            .into_any_element()
    }

    /// The scrolling repo list: `ScrollArea`'s own two ids, hand-built. See
    /// the module docs.
    fn scroll_area(&self, theme: &Theme, anchors: &dyn AnchorSink) -> AnyElement {
        let mut list = div();
        for section in &self.sections {
            list = list.child(section.render(theme, anchors));
        }

        let viewport = anchors.boxed(
            AnchorId::from(scroll_area::ID_VIEWPORT),
            div().w_full().h_full().overflow_hidden().child(list),
        );

        anchors.boxed(
            AnchorId::from(scroll_area::ID_ROOT),
            div()
                .relative()
                .flex_1()
                .min_h(px(0.0))
                .w(self.scroll_width)
                .h(self.scroll_height)
                .child(viewport),
        )
    }

    /// Renders the tree, opting every contract anchor into `anchors`.
    #[must_use]
    pub fn render(&self, theme: &Theme, anchors: &dyn AnchorSink) -> AnyElement {
        anchors.root(
            AnchorId::from(ID_ROOT),
            div()
                .flex()
                .flex_col()
                .flex_1()
                .overflow_hidden()
                .child(self.home_row(theme, anchors))
                .child(self.scroll_area(theme, anchors)),
        )
    }
}

#[cfg(test)]
mod tests {
    use super::{ID_ROOT, WorkspaceTree};
    use crate::theme::Theme;
    use gpui::px;

    #[test]
    fn the_root_id_is_workspace_tree() {
        assert_eq!(ID_ROOT, "workspace-tree");
    }

    #[test]
    fn the_fixture_is_one_repo_at_the_live_scroll_area_extent() {
        let theme = Theme::DARK;
        let tree = WorkspaceTree::fixture(&theme);
        assert_eq!(tree.sections.len(), 1);
        assert!(!tree.project_home.is_active);
        assert_eq!(tree.scroll_width, px(344.0));
        assert_eq!(tree.scroll_height, px(936.0));
    }
}
