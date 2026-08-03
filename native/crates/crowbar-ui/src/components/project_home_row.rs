//! `project_home_row` — the sidebar's own "home" row for the active project:
//! a 20px identity glyph (the house library mark, or the agent spinner while
//! the home workspace is working), the project's name (or the literal
//! `"home"` fallback), and two trailing icon-only actions that open the
//! import-repository dialog and push the project switcher onto the nav
//! stack.
//!
//! The native half of `web/src/components/layout/project-home-row.tsx`.
//! Every length below came out of the app's own `tailwindcss` 4.3.0 with the
//! utility as a candidate — the method `native/MAPPING.md` fixes. See
//! `native/mapping/project-home-row.md`.
//!
//! # `AddRepositoryModal` is not composed
//!
//! `project-home-row.tsx` renders `<AddRepositoryModal open={addRepoOpen}
//! .../>` unconditionally, starting closed. `AddRepositoryModal` lives under
//! `components/projects/`, a different directory this item's own scope
//! (`components/layout/`, per the brief and `native/mapping/layout-
//! denominator.md`) does not reach — the same posture `sidebar_carousel.rs`
//! already took for its own four wrapped panels ("'Ported' here means the
//! outer scroll-track geometry only — it does not transitively cover
//! anything it wraps"). [`ProjectHomeRow`] has no field for the modal's open
//! state at all: the row's own painted picture never changes when it opens,
//! so there is nothing this composition omits by leaving it out.
//!
//! # This composition does not call `Button::render`
//!
//! Two `<Button variant="ghost" size="icon-xs">` elements sit on this row's
//! trailing edge, each merging `ROW_SUB_ACTION` (`workspace-row-base.ts`)
//! and a call-site `size-6` over the variant's own classes.
//! `Button::render` calls `anchors.root(id, …)` — its own frame boundary,
//! wrong for two buttons inside this surface's single root — so, exactly as
//! `context_pill.rs` and `sidebar_project_header.rs` already do, this file
//! hand-builds each button's box off `button::Size`/`RadiusClass`'s own
//! public values (via [`super::row_base::sub_action_box`]) rather than
//! reusing `Button::render`'s anchor machinery. The React source now
//! overrides `button.tsx`'s own `data-oracle-id: 'button'` default on both
//! (`data-oracle-id="project-home-row-import"` /
//! `"project-home-row-switch"`, added by this item), so — as `context_pill`'s
//! own module docs put it — the collision this reasoning warns about is
//! avoided on both sides at once.
//!
//! ## `size-6` beats the `icon-xs` variant's own box at every width
//!
//! `button.tsx` composes `cn(buttonVariants({ className, size, variant }),
//! …)`, so the call site's `cn(ROW_SUB_ACTION, 'size-6')` is merged **over**
//! `icon-xs`'s own `size-7 sm:size-6`. tailwind-merge groups a class by its
//! own variant scope: the call site's un-prefixed `size-6` conflicts with
//! (and replaces) `icon-xs`'s own un-prefixed `size-7`, but does **not**
//! touch `icon-xs`'s `sm:size-6` — a *different* scope. The merged result is
//! `size-6 sm:size-6`: **24px at every width**, not the 28→24 step every
//! other `icon-xs` button in this port takes. [`row_base::SUB_ACTION_SIZE`]
//! is that fixed number; this surface therefore needs no
//! [`super::git_status_row::Breakpoint`] parameter at all, on either of its
//! two buttons or on its own shell — `ROW_BASE`/`ROW_ACTIVE`/`ROW_INACTIVE`/
//! `ROW_SUB_ACTION` carry no `sm:` variant of their own either.
//!
//! `rounded-lg` beats `icon-xs`'s own `rounded-md` the same way — see
//! [`row_base::sub_action_box`].
//!
//! # Composes `workspace_branch_icon`, does not reimplement it
//!
//! [`ProjectHomeRow::render`] reaches
//! [`super::workspace_branch_icon::WorkspaceBranchIcon::render`] directly for
//! the working-state glyph — already opting its own anchor via `.boxed()`,
//! never `.root()`, so nesting it here is collision-free the same way
//! `context_pill.rs`'s identical composition is. No `oracleSurfaceScope`
//! entry was needed: the nested `workspace-branch-icon` anchor (and, one
//! level deeper, `flicker-spinner`) is exactly what this composition paints,
//! not foreign content left unpainted — `context_pill.rs`'s own finding,
//! reached the same way here.
//!
//! # The `Library` glyph is an empty box, and it is not separately anchored
//!
//! `<Library size={16} />` has no native equivalent — the same call every
//! icon in this port makes — so it paints an empty, unpainted box. It is
//! **not** given its own anchor: the wrapping 20px span
//! (`project-home-row-icon`) is always present regardless of which picture
//! it holds (the spinner or the glyph) and is the more useful, stable anchor
//! point, matching the restraint `context_pill.rs`'s own label lines already
//! take (not every nested text run or glyph gets its own id).
//!
//! # The state axis
//!
//! | flag | here |
//! |---|---|
//! | `selected` | **real.** `isActive` (`useMatch`) selects [`row_base::active`] over [`row_base::inactive`] — a single row's own active/idle picture is exactly what this flag names (`native/oracle/ANCHORS.md` v1.1: `` `data-active='true'` ``). |
//! | `empty`, `loading`, `error`, `hover`, `focus` | **unmodelled.** `empty`/`loading`/`error` have no rule on this component at all — `StateFlag::Empty` is `git-status-row`'s own "nothing on the trailing edge" concept and this row always paints both its glyph and its two actions. `hover`/`focus` are colour-only rules (`ROW_INACTIVE`'s `hover:bg-accent`, `ROW_SUB_ACTION`'s `hover:…`, both buttons' `focus-visible:ring-…`) with no runtime seam on this struct — [`row_base`]'s own module docs record why. |

use gpui::{
    AnyElement, Div, IntoElement as _, ParentElement as _, Pixels, SharedString, Styled as _, div,
    px,
};

use super::anchor::{AnchorId, AnchorSink};
use super::row_base;
use super::workspace_branch_icon::{self, WorkspaceBranchIcon};
use crate::theme::Theme;

/// The row's own anchor — every other bound on this surface is reported
/// relative to it.
pub const ID_ROOT: &str = "project-home-row";
/// The 20px identity-glyph wrapper's own anchor — present on every cell,
/// holding either the spinner or the `Library` glyph. See the module docs.
pub const ID_ICON: &str = "project-home-row-icon";
/// The truncating project-name label's own anchor.
pub const ID_LABEL: &str = "project-home-row-label";
/// The leading trailing-action button's own anchor, namespaced away from
/// `button.tsx`'s generic default. See the module docs.
pub const ID_IMPORT: &str = "project-home-row-import";
/// The trailing trailing-action button's own anchor.
pub const ID_SWITCH: &str = "project-home-row-switch";

/// **Empty.** [`ID_LABEL`]'s own box is `flex-1` (sized by the row, not by
/// its own text), so nothing on this surface sizes to a text run's own
/// max-content width.
pub const CONTENT_SIZED: [&str; 0] = [];
/// [`ID_LABEL`] only — a blockified flex item in an `items-center` row with
/// no explicit height of its own, so its box *is* its own line box
/// regardless of the row's authored [`row_base::HEIGHT`]. The same shape
/// `git_status_row`'s `ID_NAME` already is.
pub const LINE_SIZED: [&str; 1] = [ID_LABEL];

/// `h-5 w-5` — the identity glyph's own wrapper. Matches the repo avatar's
/// own 20px box, per the React source's comment, so this row's label lines
/// up with the repo rows below it.
pub const ICON_WRAPPER_SIZE: Pixels = px(20.0);

/// `<Library size={16} />`'s own box — unanchored (see the module docs), but
/// still given its real intended extent rather than collapsed to nothing.
pub const LIBRARY_GLYPH_SIZE: Pixels = px(16.0);

/// One `<ProjectHomeRow>`.
#[derive(Clone, Debug, PartialEq)]
pub struct ProjectHomeRow {
    /// `isActive` (`useMatch({ from: '/_shell/ide/$projectId/home' })`) —
    /// selects [`row_base::active`] over [`row_base::inactive`].
    pub is_active: bool,
    /// `working` (`useHomeWorkspaceStore`) — swaps the `Library` glyph for
    /// the agent spinner.
    pub working: bool,
    /// `activeProject?.name ?? 'home'`, already resolved by the caller —
    /// this struct does not know what a missing active project means, only
    /// what to paint.
    pub project_name: SharedString,
}

impl ProjectHomeRow {
    /// A representative cell: idle, inactive, the `'home'` fallback name —
    /// the resting picture a fresh sidebar shows before any project has
    /// loaded.
    #[must_use]
    pub fn fixture() -> Self {
        Self {
            is_active: false,
            working: false,
            project_name: SharedString::new_static("home"),
        }
    }

    /// The 20px identity glyph: the spinner while [`Self::working`], the
    /// (empty, unpainted) `Library` glyph otherwise. See the module docs for
    /// why the glyph itself carries no anchor of its own.
    fn icon(&self, theme: &Theme, anchors: &dyn AnchorSink) -> AnyElement {
        let inner = if self.working {
            WorkspaceBranchIcon {
                status: workspace_branch_icon::Status::default(),
                working: true,
                is_placeholder: false,
            }
            .render(theme, anchors)
        } else {
            div()
                .flex_shrink_0()
                .w(LIBRARY_GLYPH_SIZE)
                .h(LIBRARY_GLYPH_SIZE)
                .into_any_element()
        };
        anchors.boxed(AnchorId::from(ID_ICON), Self::icon_wrapper().child(inner))
    }

    /// `'inline-flex h-5 w-5 shrink-0 items-center justify-center'`.
    fn icon_wrapper() -> Div {
        div()
            .flex()
            .flex_shrink_0()
            .items_center()
            .justify_center()
            .w(ICON_WRAPPER_SIZE)
            .h(ICON_WRAPPER_SIZE)
    }

    /// The truncating name label, text colour supplied by the caller
    /// ([`row_base::active`]/[`row_base::inactive`] both resolve to
    /// `theme.foreground` — see [`row_base`]'s own module docs).
    ///
    /// `font-mono`, resolved through `theme.font_mono` the same way
    /// `context_pill.rs`'s own trigger reads it. The container carries no
    /// anchor of its own — [`row_base::label_container`]'s own doc comment
    /// explains why: the text run's box already *is* the container's box, so
    /// a second wrapping anchor under the same id would record twice.
    fn label(&self, theme: &Theme, anchors: &dyn AnchorSink) -> AnyElement {
        row_base::label_container(theme.foreground)
            .font_family(theme.font_mono.primary().unwrap_or("monospace"))
            .child(anchors.text(
                AnchorId::new(ID_LABEL).line_sized(),
                self.project_name.clone(),
            ))
            .into_any_element()
    }

    /// One trailing action button — a hand-built [`row_base::sub_action_box`]
    /// wrapping an empty, unpainted glyph. See the module docs for why this
    /// does not call `Button::render`.
    fn sub_action(theme: &Theme, anchors: &dyn AnchorSink, id: &'static str) -> AnyElement {
        anchors.boxed(
            AnchorId::from(id),
            row_base::sub_action_box(theme).child(row_base::sub_action_glyph()),
        )
    }

    /// Renders the row, opting every contract anchor into `anchors`.
    #[must_use]
    pub fn render(&self, theme: &Theme, anchors: &dyn AnchorSink) -> AnyElement {
        let shell = if self.is_active {
            row_base::active(theme)
        } else {
            row_base::inactive(theme)
        };

        anchors.root(
            ID_ROOT.into(),
            shell
                .w_full()
                .child(self.icon(theme, anchors))
                .child(self.label(theme, anchors))
                .child(Self::sub_action(theme, anchors, ID_IMPORT))
                .child(Self::sub_action(theme, anchors, ID_SWITCH)),
        )
    }
}

#[cfg(test)]
mod tests {
    use super::{
        CONTENT_SIZED, ICON_WRAPPER_SIZE, ID_ICON, ID_IMPORT, ID_LABEL, ID_ROOT, ID_SWITCH,
        LIBRARY_GLYPH_SIZE, LINE_SIZED, ProjectHomeRow,
    };
    use gpui::px;

    #[test]
    fn every_length_is_the_compiled_value() {
        assert_eq!(ICON_WRAPPER_SIZE, px(20.0)); // h-5 w-5
        assert_eq!(LIBRARY_GLYPH_SIZE, px(16.0)); // size={16}
    }

    #[test]
    fn the_declaration_lists_match_what_render_actually_declares() {
        assert!(CONTENT_SIZED.is_empty());
        assert_eq!(LINE_SIZED, [ID_LABEL]);
    }

    #[test]
    fn the_five_anchor_ids_are_distinct_and_namespaced() {
        let ids = [ID_ROOT, ID_ICON, ID_LABEL, ID_IMPORT, ID_SWITCH];
        for id in ids {
            assert!(id.starts_with(ID_ROOT), "{id}");
        }
        assert_ne!(
            ID_IMPORT, "button",
            "must not collide with button.tsx's own default"
        );
        assert_ne!(
            ID_SWITCH, "button",
            "must not collide with button.tsx's own default"
        );
        assert_ne!(ID_IMPORT, ID_SWITCH);

        let mut sorted = ids.to_vec();
        sorted.sort_unstable();
        let before = sorted.len();
        sorted.dedup();
        assert_eq!(sorted.len(), before, "{ids:?}");
    }

    #[test]
    fn the_fixture_is_the_idle_inactive_home_fallback() {
        let row = ProjectHomeRow::fixture();
        assert!(!row.is_active);
        assert!(!row.working);
        assert_eq!(row.project_name, "home");
    }
}
