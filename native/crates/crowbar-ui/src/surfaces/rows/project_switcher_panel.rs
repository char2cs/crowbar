//! `project_switcher_panel` — the body of the pushed "Projects" sidebar
//! screen: a column of project rows (each the project's own name, tinted
//! when it is the active one) followed by a static "Import project" row.
//!
//! The native half of `web/src/components/layout/project-switcher-panel.tsx`.
//! `nav-stack.tsx` supplies the back button and the screen title around it —
//! out of this item's scope, per `native/mapping/layout-denominator.md` §4
//! (`nav-stack` is its own, separately-flagged Tier B target). Every length
//! below came out of the app's own `tailwindcss` 4.3.0 with the utility as a
//! candidate — the method `native/MAPPING.md` fixes. See
//! `native/mapping/project-switcher-panel.md`.
//!
//! # `ImportProjectModal` is not composed
//!
//! `project-switcher-panel.tsx` renders `<ImportProjectModal open={importOpen}
//! .../>` unconditionally, starting closed. `ImportProjectModal` lives under
//! `components/projects/`, outside this item's scope — the same
//! `sidebar_carousel.rs` posture [`super::project_home_row`]'s own module
//! docs already cite for `AddRepositoryModal`. This panel's own painted
//! picture never depends on whether the dialog is open, so nothing is
//! omitted by leaving it out.
//!
//! # Row ids are index-parameterized, not fixed strings
//!
//! The project list's length varies (zero projects up through however many
//! are open), and `native/oracle/ANCHORS.md` v1.8 refuses a capture where
//! the same anchor id appears twice under one root — `sidebar_project_header
//! .rs`'s own four-button fix, here because the row *count* varies rather
//! than a fixed small set of named buttons. [`ProjectSwitcherPanel::render`]
//! therefore builds each row's id as `project-switcher-panel-row-{index}`
//! (and its label as `…-{index}-label`), matching the same
//! `data-oracle-id={`project-switcher-panel-row-${index}`}` template literal
//! this item adds to the React source. There is no static `LINE_SIZED`
//! array naming them for this reason — a fixed-size array cannot enumerate a
//! parameterized family — but every row's own label is still declared
//! `line_sized` at its own call site, the declaration `AnchorId` carries
//! regardless of whether a `pub const` also lists it.
//!
//! # `h-full` on the root is not modelled
//!
//! `project-switcher-panel.tsx`'s own outer `<div className="flex h-full
//! flex-col">` stretches to whatever height its `NavStack` parent gives it.
//! Measured against this port's own row-layout harness
//! (`row_layout.rs`'s `Stage`): the immediate parent of a surface's root is
//! `div().w(width_px)` with **no height style at all** — `display: block`,
//! auto height — so a percentage height here resolves the way CSS resolves
//! any percentage against an indefinite-height containing block: as `auto`.
//! `h-full` therefore collapses to exactly the same picture as omitting it,
//! and this port omits it rather than calling `.h_full()` for a property
//! that would be a no-op in this harness and a live risk (an indefinite
//! ancestor resolving a percentage to `0` instead of `auto`) everywhere
//! else. [`ProjectSwitcherPanel::content_height`] is what drives the
//! surface's own window height instead, the same shape `command.rs`'s own
//! `popup_height` already is.
//!
//! # Neither project row nor the import row calls `Button::render`
//!
//! Neither is a `<Button>` in the React source — both are plain
//! `<button type="button">` elements styled directly off `ROW_BASE` — so
//! this file never faces the anchor-collision question `project_home_row
//! .rs`'s two trailing actions do. Each row is hand-built off
//! [`super::row_base`] the same way every consumer of that module is.
//!
//! # The state axis
//!
//! | flag | here |
//! |---|---|
//! | `empty` | **real.** Zero projects is a genuinely reachable picture — a fresh install with nothing imported yet — and it swaps [`ProjectSwitcherPanel::rows`] for an empty list, leaving only the always-present import row. The same shape `command.rs`'s own `Empty`/`Item` swap is. |
//! | `loading`, `error`, `hover`, `focus`, `selected` | **unmodelled.** `loading`/`error` have no rule on this component at all. `hover:bg-accent` (both row kinds) is colour-only with no runtime seam here — [`super::row_base`]'s own module docs record why. `focus`/`selected` (`aria-current`) carry no styling rule on either row: `aria-current` is an accessibility attribute, not a CSS selector target anywhere in this file's class strings. |

use gpui::{
    AnyElement, Div, FontWeight, ParentElement as _, Pixels, SharedString, Styled as _, div, px,
};

use crate::anchor::{AnchorId, AnchorSink};
use super::row_base;
use crate::theme::{Color, Theme, ui_sans_font};

/// The panel's own anchor — every other bound on this surface is reported
/// relative to it.
pub const ID_ROOT: &str = "project-switcher-panel";
/// The static "Import project" row's own anchor.
pub const ID_IMPORT: &str = "project-switcher-panel-import";
/// The import row's own label anchor.
pub const ID_IMPORT_LABEL: &str = "project-switcher-panel-import-label";

/// **Empty.** Every project-row label is `flex-1` (sized by the row, not by
/// its own text), so nothing on this surface sizes to a text run's own
/// max-content width. See the module docs for why the per-row ids
/// themselves have no static array here.
pub const CONTENT_SIZED: [&str; 0] = [];
/// [`ID_IMPORT_LABEL`] only. Every project row's own label anchor is
/// *also* declared `line_sized` at its own call site
/// ([`ProjectSwitcherPanel::project_row`]) — see the module docs for why a
/// parameterized family of ids has no fixed-size array to sit in.
pub const LINE_SIZED: [&str; 1] = [ID_IMPORT_LABEL];

/// `--spacing`, Tailwind's stock `0.25rem` at a 16px root.
const SPACING: f32 = 4.0;

/// `py-1` on the row list's own wrapper.
pub const WRAPPER_PADDING_Y: Pixels = px(SPACING);
/// `<Plus className="size-3.5 shrink-0" />`'s own box.
pub const PLUS_GLYPH_SIZE: Pixels = px(SPACING * 3.5);
/// `bg-accent/60` on the active project row.
const ACTIVE_TINT: f32 = 60.0;

/// One project row: `projects.map((p) => …)`'s own `p`.
#[derive(Clone, Debug, PartialEq)]
pub struct ProjectRow {
    /// `p.name`.
    pub name: SharedString,
    /// `p.id === activeProjectId`.
    pub is_active: bool,
}

/// The panel.
#[derive(Clone, Debug, PartialEq)]
pub struct ProjectSwitcherPanel {
    /// `projects.map(…)`'s own list — may be empty (`StateFlag::Empty`'s own
    /// cell; see the module docs), in which case only the import row paints.
    pub rows: Vec<ProjectRow>,
}

impl ProjectSwitcherPanel {
    /// A representative cell: two projects, the first one active — the live,
    /// reachable shape (at least one project always exists once the app has
    /// finished onboarding).
    #[must_use]
    pub fn fixture() -> Self {
        Self {
            rows: vec![
                ProjectRow {
                    name: SharedString::new_static("crowbar"),
                    is_active: true,
                },
                ProjectRow {
                    name: SharedString::new_static("sandbox"),
                    is_active: false,
                },
            ],
        }
    }

    /// The panel's own intrinsic content height: the wrapper's `py-1` on
    /// both edges, plus one [`row_base::HEIGHT`] (with its own
    /// [`row_base::MARGIN_Y`] on both edges) per project row and one more
    /// for the always-present import row. What
    /// [`crate::components`]-external callers use to drive this surface's
    /// window, the same shape `command.rs`'s own `popup_height` already is —
    /// see the module docs for why this, and not `h-full`, is what decides
    /// the height.
    #[must_use]
    pub fn content_height(&self) -> Pixels {
        let per_row = row_base::HEIGHT + row_base::MARGIN_Y * 2.0;
        #[expect(
            clippy::cast_precision_loss,
            reason = "row counts are small, bounded by this surface's own u8 --count option; \
                      no fractional px is ever lost in practice"
        )]
        let rows_and_import = (self.rows.len() + 1) as f32;
        WRAPPER_PADDING_Y * 2.0 + per_row * rows_and_import
    }

    /// One project row's own id: `project-switcher-panel-row-{index}`. See
    /// the module docs for why this is a function, not a `pub const`.
    #[must_use]
    pub fn row_id(index: usize) -> SharedString {
        SharedString::from(format!("project-switcher-panel-row-{index}"))
    }

    /// One project row's own label id.
    #[must_use]
    pub fn row_label_id(index: usize) -> SharedString {
        SharedString::from(format!("project-switcher-panel-row-{index}-label"))
    }

    /// One project row: `ROW_BASE` plus `border-transparent text-left
    /// hover:bg-accent`, plus `bg-accent/60 text-foreground` when active.
    /// `text-foreground` is applied unconditionally — it is the row's
    /// ambient default whether or not `isActive` restates it, the same
    /// finding [`super::project_home_row`]'s own label makes.
    fn project_row(
        theme: &Theme,
        anchors: &dyn AnchorSink,
        index: usize,
        row: &ProjectRow,
    ) -> AnyElement {
        // No `.w_full()`: this row is a flex item of a `flex-col` container
        // with no `align-items` override, so it stretches to the
        // container's own cross-axis size by default — and, unlike an
        // explicit `width: 100%`, a stretched size is computed *net of* the
        // item's own margin. `.w_full()` here was a real bug, caught by
        // `row_layout`'s own `every_row_is_inset_by_the_same_margin_on_both_
        // edges`: it reported 240px (root's own full width, margin added on
        // top and clipped/ignored rather than subtracted) against the
        // expected 228px (240 minus `row_base::MARGIN_X * 2`).
        let mut shell = row_base::base(theme)
            .border_color(Color::TRANSPARENT)
            .text_color(theme.foreground)
            .mx(row_base::MARGIN_X)
            .my(row_base::MARGIN_Y);
        if row.is_active {
            shell = shell.bg(theme.accent.mix(ACTIVE_TINT, Color::TRANSPARENT));
        }

        let label = row_base::label_container(theme.foreground)
            .font_family(theme.font_mono.primary().unwrap_or("monospace"))
            .child(anchors.text(
                AnchorId::new(Self::row_label_id(index)).line_sized(),
                row.name.clone(),
            ));

        anchors.boxed(AnchorId::new(Self::row_id(index)), shell.child(label))
    }

    /// The static "Import project" row: `ROW_BASE` plus `border-transparent
    /// text-muted-foreground hover:bg-accent hover:text-foreground`, a
    /// leading `Plus` glyph (empty, unpainted — no native equivalent) and a
    /// label that, unlike every project row's, is **not** `font-mono` — the
    /// React source carries no such class on this one span. That split does
    /// **not** need a second [`row_base::LINE_HEIGHT_RELATIVE`]: the ratio
    /// is Tailwind's own preflight default, inherited unitless, so it
    /// resolves against this label's own 13px font-*size* the same way
    /// regardless of font-*family* — see that constant's own doc comment
    /// for the full derivation and the oracle failure that corrected it.
    ///
    /// `.font_weight(FontWeight::MEDIUM)` is repeated **after**
    /// `.font(ui_sans_font(theme))` on purpose, not decoration:
    /// [`row_base::label_container`] already sets `FontWeight::MEDIUM`
    /// (`ROW_BASE`'s own `font-medium`), but gpui's `Styled::font` writes
    /// every field of the `Font` it is given — including `weight`, which
    /// `ui_sans_font`'s own `gpui::font(family)` constructor leaves at its
    /// default (`400`) — so calling `.font(...)` *after* `.font_weight(...)`
    /// silently clobbers it back to `400`. `RepoAvatar::letter_box` already
    /// gets this ordering right (`.font(...)` then `.font_weight(BOLD)`);
    /// this call site had it backwards until this fix — matched live:
    /// `native/mapping/project-switcher-panel.md`'s VERDICT recorded
    /// `import-label.font.weight: 400, expected 500` and a `text_width`
    /// shortfall from shaping the same string one weight lighter than the
    /// project row beside it, which is correct at `500` because
    /// [`Self::project_row`]'s own label uses `.font_family(&str)` (only the
    /// family, not a whole `Font`) instead of `.font(Font)` and so never
    /// clobbers the weight [`row_base::label_container`] already set.
    fn import_row(theme: &Theme, anchors: &dyn AnchorSink) -> AnyElement {
        // No `.w_full()` — see `Self::project_row`'s own comment.
        let shell = row_base::base(theme)
            .border_color(Color::TRANSPARENT)
            .text_color(theme.muted_foreground)
            .mx(row_base::MARGIN_X)
            .my(row_base::MARGIN_Y)
            .child(Self::plus_glyph());

        let label = row_base::label_container(theme.muted_foreground)
            .font(ui_sans_font(theme))
            .font_weight(FontWeight::MEDIUM)
            .child(anchors.text(
                AnchorId::from(ID_IMPORT_LABEL).line_sized(),
                SharedString::new_static("Import project"),
            ));

        anchors.boxed(AnchorId::from(ID_IMPORT), shell.child(label))
    }

    /// `<Plus className="size-3.5 shrink-0" />` — empty and unpainted, the
    /// same call every icon in this port makes.
    fn plus_glyph() -> Div {
        div().flex_shrink_0().w(PLUS_GLYPH_SIZE).h(PLUS_GLYPH_SIZE)
    }

    /// Renders the panel, opting every contract anchor into `anchors`.
    #[must_use]
    pub fn render(&self, theme: &Theme, anchors: &dyn AnchorSink) -> AnyElement {
        let mut body = div().flex().flex_col().py(WRAPPER_PADDING_Y);
        for (index, row) in self.rows.iter().enumerate() {
            body = body.child(Self::project_row(theme, anchors, index, row));
        }
        body = body.child(Self::import_row(theme, anchors));

        anchors.root(ID_ROOT.into(), div().flex().flex_col().child(body))
    }
}

#[cfg(test)]
mod tests {
    use super::{
        CONTENT_SIZED, ID_IMPORT, ID_IMPORT_LABEL, ID_ROOT, LINE_SIZED, PLUS_GLYPH_SIZE,
        ProjectRow, ProjectSwitcherPanel, WRAPPER_PADDING_Y,
    };
    use gpui::px;

    #[test]
    fn every_length_is_the_compiled_spacing_multiple_or_a_literal() {
        assert_eq!(WRAPPER_PADDING_Y, px(4.0)); // py-1
        assert_eq!(PLUS_GLYPH_SIZE, px(14.0)); // size-3.5
    }

    #[test]
    fn the_declaration_lists_match_what_render_actually_declares() {
        assert!(CONTENT_SIZED.is_empty());
        assert_eq!(LINE_SIZED, [ID_IMPORT_LABEL]);
    }

    #[test]
    fn row_ids_are_indexed_and_never_collide_with_the_import_row() {
        let a = ProjectSwitcherPanel::row_id(0);
        let b = ProjectSwitcherPanel::row_id(1);
        assert_ne!(a, b);
        assert_ne!(a, ID_IMPORT);
        assert_ne!(a, ID_ROOT);

        let al = ProjectSwitcherPanel::row_label_id(0);
        let bl = ProjectSwitcherPanel::row_label_id(1);
        assert_ne!(al, bl);
        assert_ne!(al, ID_IMPORT_LABEL);
        assert!(al.to_string().starts_with(&a.to_string()), "{al} / {a}");
    }

    /// The always-present import row means content height is never just
    /// `rows.len()`'s own multiple — a zero-row (`empty`) cell still carries
    /// one row's worth of height (the import row itself), and each
    /// additional project row costs exactly one more.
    ///
    /// **Mutation, run:** dropped the `+ 1` from `content_height`'s
    /// `rows_and_import` (`self.rows.len() as f32`, no import row counted).
    /// `cargo test -p crowbar-ui content_height_grows_by_one_row_per_project`
    /// failed as predicted: `left: 8px, right: 48px` — the empty panel
    /// computed to just the wrapper padding (`WRAPPER_PADDING_Y * 2.0` = 8),
    /// zero rows tall, exactly the floor this test exists to catch. Reverted
    /// after confirming.
    #[test]
    fn content_height_grows_by_one_row_per_project_and_floors_at_the_import_row() {
        use super::row_base;
        let per_row = row_base::HEIGHT + row_base::MARGIN_Y * 2.0;

        let empty = ProjectSwitcherPanel { rows: vec![] };
        assert_eq!(empty.content_height(), WRAPPER_PADDING_Y * 2.0 + per_row);

        let one = ProjectSwitcherPanel {
            rows: vec![ProjectRow {
                name: "crowbar".into(),
                is_active: true,
            }],
        };
        assert_eq!(one.content_height(), empty.content_height() + per_row);
    }

    #[test]
    fn the_fixture_is_two_projects_the_first_active() {
        let panel = ProjectSwitcherPanel::fixture();
        assert_eq!(panel.rows.len(), 2);
        assert!(panel.rows[0].is_active);
        assert!(!panel.rows[1].is_active);
    }
}
