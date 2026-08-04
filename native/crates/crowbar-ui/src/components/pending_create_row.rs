//! `pending_create_row` — the optimistic row shown while a workspace create is
//! in flight: a spinner plus the branch name, or — if the create failed — an
//! inline error with a dismiss button.
//!
//! The native half of `web/src/components/layout/pending-create-row.tsx`.
//! First of `native/mapping/layout-denominator.md` §8's Cluster 8 chain
//! (`pending-create-row` → `workspace-tree-item` → `repo-section` →
//! `workspace-tree`) — every consumer downstream composes this file rather
//! than restating its row. Every length below came out of the app's own
//! `tailwindcss` 4.3.0 with the utility as a candidate — the method
//! `native/MAPPING.md` fixes. See `native/mapping/pending-create-row.md`.
//!
//! # Composes [`super::row_base`], does not restate it
//!
//! `ROW_BASE` merged with `border-transparent opacity-60 pointer-events-none`
//! — [`super::row_base::base`] plus an explicit transparent border colour and
//! [`gpui::Styled::opacity`]. `pointer-events-none` has no gpui counterpart
//! (there is no hit-testing to disable) and is not modelled.
//!
//! # `mx-1.5`/`my-0.5` are modelled, and reached via stretch — not `.w_full()`
//!
//! This row always sits beside siblings (its own tempId-keyed instance is
//! one of possibly several concurrent pending creates in one list), the
//! `project_switcher_panel::ProjectRow` shape [`row_base`]'s own module docs
//! describe, not `project_home_row`'s standalone one — so
//! [`row_base::MARGIN_X`]/[`row_base::MARGIN_Y`] are applied here.
//! [`PendingCreateRow::render`]'s own padding wrapper is `flex flex-col` so
//! the row stretches to fill it **net of its own margin** — `.w_full()`
//! would set `width: 100%` unconditionally and NOT shrink for the margin the
//! way flex stretch does, overflowing the wrapper by `2 * MARGIN_X`. This is
//! `project_switcher_panel.rs`'s own documented bug, avoided here by
//! following its fix rather than rediscovering the failure.
//!
//! # This surface is always [`AnchorSink::boxed`], never [`AnchorSink::root`]
//!
//! `pending-create-row.tsx` is rendered by **both**
//! `workspace-tree-item.tsx` and `repo-section.tsx` (`native/mapping/
//! layout-denominator.md` §8), so [`PendingCreateRow::render`] opts its own
//! root anchor in via [`AnchorSink::boxed`] — never [`AnchorSink::root`],
//! which clears every anchor recorded so far and would wipe out whichever
//! parent row composed it. The same posture [`super::workspace_branch_icon`]
//! already takes for the identical reason, restated in its own module docs.
//!
//! # The spinner composes [`super::workspace_branch_icon`] directly; the
//! error mark does not
//!
//! The non-error branch renders `<WorkspaceAgentSpinner />`, which
//! `workspace-branch-icon.tsx`'s own module docs confirm is exactly
//! `WorkspaceBranchIcon{ working: true }` under its own shared
//! `workspace-branch-icon` anchor. This file reuses that composition
//! directly — safe here because [`PendingCreateRow`] is a leaf with no
//! internal repetition of its own (unlike `workspace-tree-item.rs`, which
//! cannot reuse [`super::workspace_branch_icon::WorkspaceBranchIcon::render`]
//! for the identical reason it gives in its own module docs). The error
//! branch's `✕` mark is not `WorkspaceBranchIcon`'s picture at all — a
//! genuinely different glyph in the same slot — so it is hand-painted under
//! [`ID_ICON`], the row's own id, mutually exclusive with the composed one.
//!
//! # The outer padding wrapper carries no anchor
//!
//! `<div style={{ paddingLeft }}>` is a plain positioning wrapper; the row
//! inside it already reports its own absolute origin including that offset,
//! so a second anchor on the wrapper would not tell the differ anything the
//! row's own bounds do not already carry — see the React diff's own comment
//! on this file.
//!
//! # The state axis
//!
//! | flag | here |
//! |---|---|
//! | `loading`, `error`, `hover`, `focus`, `selected`, `empty` | **unmodelled.** This row has no selection/hover/focus rule of its own (`pointer-events-none`, in fact) and no `StateFlag::Empty`-shaped trailing-edge concept. `error` here names [`PendingCreateRow::error`], a domain field (`pending.error`) entirely orthogonal to `StateFlag::Error` — the §8.3 flag stays mandatorily unmodelled, per `crowbar-app/src/surface.rs`'s own invariant, on every surface in this port including this one. |

use gpui::{
    AnyElement, Div, IntoElement as _, ParentElement as _, Pixels, SharedString, Styled as _, div,
    px,
};

use super::anchor::{AnchorId, AnchorSink};
use super::row_base;
use super::workspace_branch_icon::{self, WorkspaceBranchIcon};
use crate::theme::{Color, Theme};

/// The row's own anchor — every other bound on this surface is reported
/// relative to it. See the module docs for why it is `.boxed`, not `.root`.
pub const ID_ROOT: &str = "pending-create-row";
/// The leading icon slot: the composed `workspace-branch-icon` spinner, or —
/// mutually exclusive — this row's own `✕` error mark. See the module docs.
pub const ID_ICON: &str = "pending-create-row-icon";
/// The truncating branch-name label.
pub const ID_LABEL: &str = "pending-create-row-label";
/// The `"failed"` caption. Error branch only.
pub const ID_STATUS: &str = "pending-create-row-status";
/// The dismiss `✕` button. Error branch only.
pub const ID_DISMISS: &str = "pending-create-row-dismiss";

/// **Empty.** Every box on this surface is authored or `flex-1`; nothing
/// sizes to a text run's own max-content width except [`ID_DISMISS`], which
/// is declared inline at its own call site rather than listed here (a single
/// entry earns no array).
pub const CONTENT_SIZED: [&str; 0] = [];
/// [`ID_LABEL`] and [`ID_STATUS`] — both blockified flex items in an
/// `items-center` row with no explicit height of their own, the same shape
/// [`super::project_home_row::LINE_SIZED`] already documents.
pub const LINE_SIZED: [&str; 2] = [ID_LABEL, ID_STATUS];

/// `opacity-60`.
pub const ROW_OPACITY: f32 = 0.6;
/// `size-4` — the icon slot's own box, shared by the spinner and the error
/// mark.
pub const ICON_SIZE: Pixels = px(16.0);
/// `text-xs` (both the error mark and the `"failed`"/dismiss captions).
pub const TEXT_XS: Pixels = px(12.0);
/// `ml-1` on the dismiss button.
pub const DISMISS_MARGIN_LEFT: Pixels = px(4.0);

/// One `<PendingCreateRow>`.
#[derive(Clone, Debug, PartialEq)]
pub struct PendingCreateRow {
    /// `pending.branch`.
    pub branch: SharedString,
    /// `pending.error` — truthy swaps the spinner + label for the `✕` mark +
    /// label + `"failed"` + dismiss button.
    pub error: bool,
    /// The caller's own `paddingLeft` prop — `14` at the repo root
    /// (`repo-section.tsx`) or `(depth + 2) * 14` nested under a parent
    /// workspace (`workspace-tree-item.tsx`). A free variable of the
    /// component, not derived here.
    pub padding_left: Pixels,
}

impl PendingCreateRow {
    /// A representative cell: idle (not failed), nested one level under a
    /// parent workspace — `(0 + 2) * 14`, `workspace-tree-item.tsx`'s own
    /// shape, the more common of the two real call sites since every repo
    /// has at most one root-level create in flight at a time but may have
    /// several nested ones.
    #[must_use]
    pub fn fixture() -> Self {
        Self {
            branch: SharedString::new_static("feature/example"),
            error: false,
            padding_left: px(28.0),
        }
    }

    /// The leading icon slot.
    fn icon(&self, theme: &Theme, anchors: &dyn AnchorSink) -> AnyElement {
        if self.error {
            anchors.boxed_text(
                AnchorId::new(ID_ICON),
                Self::icon_wrapper()
                    .text_size(TEXT_XS)
                    .text_color(theme.destructive),
                SharedString::new_static("\u{2715}"),
            )
        } else {
            WorkspaceBranchIcon {
                status: workspace_branch_icon::Status::default(),
                working: true,
                is_placeholder: false,
            }
            .render(theme, anchors)
        }
    }

    /// `'flex size-4 shrink-0 items-center justify-center'`.
    fn icon_wrapper() -> Div {
        div()
            .flex()
            .flex_shrink_0()
            .items_center()
            .justify_center()
            .w(ICON_SIZE)
            .h(ICON_SIZE)
    }

    /// The truncating branch-name label — `text-muted-foreground` on both
    /// branches.
    fn label(&self, theme: &Theme, anchors: &dyn AnchorSink) -> AnyElement {
        row_base::label_container(theme.muted_foreground)
            .font_family(theme.font_mono.primary().unwrap_or("monospace"))
            .child(anchors.text(AnchorId::new(ID_LABEL).line_sized(), self.branch.clone()))
            .into_any_element()
    }

    /// The `"failed"` caption. Error branch only. An associated function,
    /// not a method: nothing about it reads `self` — the caption text is
    /// fixed — the same `clippy::unused_self` shape
    /// [`row_base::sub_action_glyph`] already is.
    fn status(theme: &Theme, anchors: &dyn AnchorSink) -> AnyElement {
        div()
            .text_size(TEXT_XS)
            .text_color(theme.destructive)
            .child(anchors.text(
                AnchorId::new(ID_STATUS).line_sized(),
                SharedString::new_static("failed"),
            ))
            .into_any_element()
    }

    /// The dismiss `✕` button. Error branch only. Content-sized: unlike
    /// every other box on this surface, nothing about it is authored beyond
    /// `ml-1` — its own box is its own text run's advance width. An
    /// associated function for the same reason [`Self::status`] is.
    fn dismiss(theme: &Theme, anchors: &dyn AnchorSink) -> AnyElement {
        anchors.boxed_text(
            AnchorId::new(ID_DISMISS).content_sized(),
            div()
                .ml(DISMISS_MARGIN_LEFT)
                .text_size(TEXT_XS)
                .text_color(theme.muted_foreground),
            SharedString::new_static("\u{2715}"),
        )
    }

    /// Renders the row, opting every contract anchor into `anchors`.
    #[must_use]
    pub fn render(&self, theme: &Theme, anchors: &dyn AnchorSink) -> AnyElement {
        // No `.w_full()` — see the module docs. The row stretches to fill
        // the `flex flex-col` wrapper below, net of its own `mx`/`my`.
        let mut row = row_base::base(theme)
            .border_color(Color::TRANSPARENT)
            .opacity(ROW_OPACITY)
            .mx(row_base::MARGIN_X)
            .my(row_base::MARGIN_Y)
            .child(self.icon(theme, anchors))
            .child(self.label(theme, anchors));

        if self.error {
            row = row
                .child(Self::status(theme, anchors))
                .child(Self::dismiss(theme, anchors));
        }

        div()
            .flex()
            .flex_col()
            .pl(self.padding_left)
            .child(anchors.boxed(AnchorId::from(ID_ROOT), row))
            .into_any_element()
    }
}

#[cfg(test)]
mod tests {
    use super::{
        CONTENT_SIZED, DISMISS_MARGIN_LEFT, ICON_SIZE, ID_DISMISS, ID_ICON, ID_LABEL, ID_ROOT,
        ID_STATUS, LINE_SIZED, PendingCreateRow, ROW_OPACITY, TEXT_XS,
    };
    use gpui::px;

    #[test]
    fn every_length_is_the_compiled_value() {
        assert_eq!(ICON_SIZE, px(16.0)); // size-4
        assert_eq!(TEXT_XS, px(12.0)); // text-xs
        assert_eq!(DISMISS_MARGIN_LEFT, px(4.0)); // ml-1
        assert!((ROW_OPACITY - 0.6).abs() < f32::EPSILON); // opacity-60
    }

    #[test]
    fn the_declaration_lists_match_what_render_actually_declares() {
        assert!(CONTENT_SIZED.is_empty());
        assert_eq!(LINE_SIZED, [ID_LABEL, ID_STATUS]);
    }

    #[test]
    fn the_five_anchor_ids_are_distinct_and_namespaced() {
        let ids = [ID_ROOT, ID_ICON, ID_LABEL, ID_STATUS, ID_DISMISS];
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
    fn the_fixture_is_idle_and_nested_one_level() {
        let row = PendingCreateRow::fixture();
        assert!(!row.error);
        assert_eq!(row.branch, "feature/example");
        assert_eq!(row.padding_left, px(28.0));
    }
}
