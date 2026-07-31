//! The **stateful** gate surface: one row of the file explorer tree.
//!
//! The native half of
//! `web/src/features/file-explorer/file-explorer/components/file-explorer-tree-item.tsx`
//! rendered through `components/ui/tree-row.tsx` — the same `TreeRow` base
//! [`super::git_status_row`] uses, and deliberately so. What is different is the
//! only thing this surface exists for:
//!
//! > **This row actually enters the §8.3 state axis.** `SidebarTreeRow` takes an
//! > `active` prop that no live consumer passes, so `data-active` is dead on the
//! > git status row and every `selected` cell of the matrix compares a resting
//! > row against a resting row. The file explorer row sets `data-active` from
//! > `isActive` on every render, and it is inside `.file-tree-container`, so the
//! > *scoped* half of `file-explorer-tree.css` applies to it as well.
//!
//! # What the scoped rules change, and why this is not `sidebar_tree::row_button`
//!
//! [`super::sidebar_tree`]'s module docs record which of that stylesheet's rules
//! are unscoped — `.file-tree-item`'s width, the `::before` group, and
//! `.file-tree-row:hover` — and note that the rest is scoped to
//! `.file-tree-container`, **which the git status panel is not inside**. This
//! surface *is*, so four more rules fire on the button and none of them fire on
//! the git row:
//!
//! ```text
//! .file-tree-container .file-tree-item button {
//!   box-sizing: border-box !important;
//!   border: 1px solid transparent !important;   /* the git row has NO border */
//!   border-radius: 2px !important;              /* beats rounded-md's 8px    */
//!   background-color: transparent !important;
//!   width: 100% !important; min-width: 100% !important;
//!   display: flex !important; justify-content: flex-start !important;
//! }
//! .file-tree-container .file-tree-item button:focus-visible {
//!   border-color: color-mix(in srgb, var(--accent) 42%, var(--border)) !important;
//! }
//! ```
//!
//! The last one is the reason [`RowState::focused`] paints something here and
//! nothing on the git row: a 1px border whose colour moves off `transparent`.
//! `native/oracle/ANCHORS.md` v1.3 compares `border.color` only where
//! `border.w > 0`, and here it is exactly 1 — so focus is a **comparable** cell
//! on this surface rather than a vacuous one.
//!
//! # Two numbers that are *not* the git row's, and were checked rather than assumed
//!
//! * **The indent step is 16, not 14.** It is `settings.fileTreeIndentSize`,
//!   whose default is `16` (`web/src/features/settings/config/default-settings.ts`).
//!   The sidebar tree's own `SIDEBAR_TREE_INDENT_SIZE` is 14 and is a different
//!   constant; reusing [`super::sidebar_tree::INDENT_SIZE`] here would put every
//!   guide and the whole leading padding two pixels per level out.
//! * **The line height is `text-sm`'s, not `leading-[1.35]`.** `GitFileItem`
//!   authors `leading-[1.35]` on its row; this one authors nothing, so the line
//!   height is whatever `text-sm` sets. Compiling the app's own `index.css`
//!   through its own Tailwind (4.3.0) gives
//!
//!   ```text
//!   .text-sm { font-size: var(--text-sm);
//!              line-height: var(--tw-leading, var(--text-sm--line-height)) }
//!   --text-sm: 0.875rem;  --text-sm--line-height: calc(1.25 / 0.875);
//!   ```
//!
//!   and nothing in this app redefines either. So it is 14px text on a 20px line
//!   box, where the git row is 14px on 18.9.

use gpui::{
    AnyElement, Div, FontWeight, IntoElement as _, ParentElement as _, Pixels, SharedString,
    Styled as _, div, px, relative,
};

use super::anchor::{AnchorId, AnchorSink};
use super::sidebar_tree::{self, BASE_INDENT, GUIDE_SHIFT, ICON_SIZE, ITEM_RADIUS, ROW_HEIGHT};
use super::sidebar_tree::{GuideInset, RowState, guide_inset};
use crate::theme::{Color, Theme};

/// The root anchor: the `.file-tree-item` wrapper. Every other bound is reported
/// relative to it (`native/oracle/ANCHORS.md` §4), and it is the layer whose
/// `::before` paints hover and selection.
pub const ID_ITEM: &str = "file-row-item";
/// The `<button>` — `TreeRow`, carrying the border that focus moves.
pub const ID_BUTTON: &str = "file-row-button";
/// The file-type icon's 14px box.
pub const ID_ICON: &str = "file-row-icon";
/// The file name span.
pub const ID_NAME: &str = "file-row-name";

/// The anchors on this row whose boxes size to their own text
/// (`native/oracle/ANCHORS.md` v1.5).
///
/// One, and it is the name. The span is a flex item with `flex: 0 1 auto` and
/// no basis, so while the row has room its used width **is** its max-content
/// width — the fractional advance in `WebKit`, `ceil()`ed by gpui. That is
/// exactly the one-directional error v1.5 exists for.
///
/// **It stays correct in the `overflow` cell**, which is the question worth
/// asking before declaring a shrinkable box content-sized: there the span is
/// clamped to the space left by the icon and the paddings, all of which are
/// whole pixels, so the reference width is an integer and `ceil` of it is
/// itself. The declaration therefore never moves the target away from a number
/// the engine can hit.
///
/// The corresponding React declaration is `data-oracle-content-sized` in
/// `file-explorer-tree-item.tsx`, and the two lists have to stay identical: a
/// declaration on one side only is a `FieldPresence` delta that forgives
/// nothing.
pub const CONTENT_SIZED: [&str; 1] = [ID_NAME];

/// The anchors on this row whose **box height is their own line box**
/// (`native/oracle/ANCHORS.md` v1.6).
///
/// The name span, and only it. It is a blockified flex item holding one line of
/// text with no authored height, so its border box is the line box.
///
/// **The button is deliberately absent, and it is this surface's version of the
/// badge trap.** It paints text — it is the element the font is declared on —
/// but `h-6` pins its border box at 24px around a 20px line box. Declaring it
/// would compare 24 against 20 and manufacture a 4px delta on an anchor where
/// both engines agree exactly.
pub const LINE_SIZED: [&str; 1] = [ID_NAME];

/// One indent guide, per level: `file-row-guide-0`, `file-row-guide-1`, …
///
/// Zero-based, matching the `level` index the React `left` calculation uses, so
/// `file-row-guide-0` is the outermost guide on both sides.
#[must_use]
pub fn guide_id(level: u16) -> SharedString {
    SharedString::from(format!("file-row-guide-{level}"))
}

/// `settings.fileTreeIndentSize` — one nesting level's worth of leading padding.
///
/// **16, not the sidebar tree's 14.** The file explorer reads this off the
/// settings store (`FileExplorerTree` passes `indentSize={settings.fileTreeIndentSize}`)
/// and the default is 16. A row measured against a reference at the default
/// setting has to use the same number, and the two constants being neighbours
/// in the same crate is exactly how they would come to be confused.
pub const INDENT_SIZE: Pixels = px(16.0);

/// Tailwind's stock `--text-sm--line-height`, `calc(1.25 / 0.875)`.
///
/// Written as the division rather than as `1.4285714` because that is how the
/// stylesheet writes it, and because the pair it is derived from is the pair
/// this row's font size comes from.
const LINE_HEIGHT: f32 = 1.25 / 0.875;

/// `color-mix(in srgb, var(--accent) 42%, var(--border))` — the `:focus-visible`
/// border.
const FOCUS_BORDER_ACCENT: f32 = 42.0;

/// `.file-tree-container .file-tree-item button { border: 1px solid transparent }`.
pub const BUTTON_BORDER: Pixels = px(1.0);

/// The row's leading padding at `depth` — `FILE_TREE_BASE_INDENT + depth × 16`.
///
/// Padding rather than a spacer, for the reason `TreeRow` gives: it is what lets
/// the wrapper's background span the full width.
#[must_use]
pub fn leading_padding(depth: u16) -> Pixels {
    BASE_INDENT + INDENT_SIZE * f32::from(depth)
}

/// Where a guide's 7px host box starts, with `translateX(-3px)` folded in.
///
/// The same shape as [`super::sidebar_tree::guide_left`] over this surface's
/// indent step. It is a separate function rather than a parameter on that one
/// because the two surfaces genuinely have two different steps, and a shared
/// helper taking the step would read as though either value were a choice.
#[must_use]
pub fn guide_left(level: u16, icon_offset: Pixels) -> Pixels {
    BASE_INDENT + INDENT_SIZE * f32::from(level) + icon_offset - GUIDE_SHIFT
}

/// The button's border colour, which is the whole of what focus paints here.
///
/// `transparent` at rest — the base rule's own colour, not an absence: the
/// border is 1px in every state, so `native/oracle/ANCHORS.md` v1.3 compares its
/// colour in every state too.
#[must_use]
pub fn button_border_color(theme: &Theme, state: RowState) -> Color {
    if state.focused {
        theme.accent.mix(FOCUS_BORDER_ACCENT, theme.border)
    } else {
        Color::TRANSPARENT
    }
}

/// One row of the file explorer tree.
#[derive(Clone, Debug, PartialEq, Eq)]
pub struct FileTreeRow {
    /// The file's display name — the row paints this and nothing else.
    pub name: SharedString,
    /// `depth`.
    pub depth: u16,
    /// The depth of the row above, which caps the guides at the top of a run.
    pub previous_depth: u16,
    /// The depth of the row below, which caps them at the bottom.
    pub next_depth: u16,
    /// The visual state, as a parameter — see [`RowState`]. gpui resolves a
    /// `.hover(…)` refinement from runtime interaction state the extractor
    /// cannot see, so a row that expressed its states that way would report its
    /// resting paint in every cell of the state axis.
    pub state: RowState,
}

impl FileTreeRow {
    /// A row for one of the matrix's content lengths.
    ///
    /// The strings are [`super::ContentLength`]'s own, taken through
    /// [`super::split_path`] so that the two gate surfaces show the **same**
    /// name at the same content length. That is not decoration: a difference in
    /// truncation between the two surfaces is then a difference in the row and
    /// not in the fixture.
    #[must_use]
    pub fn fixture(content: super::ContentLength) -> Self {
        let (name, _) = super::split_path(content.path());
        Self {
            name: SharedString::from(name.to_owned()),
            depth: 0,
            previous_depth: 0,
            next_depth: 0,
            state: RowState::resting(),
        }
    }

    /// Renders the row, opting every contract anchor into `anchors`.
    #[must_use]
    pub fn render(&self, theme: &Theme, anchors: &dyn AnchorSink) -> AnyElement {
        let content: Vec<AnyElement> = vec![
            anchors.boxed(ID_ICON.into(), Self::icon(theme)),
            self.label(theme, anchors).into_any_element(),
        ];

        let button = anchors.boxed(ID_BUTTON.into(), self.button(theme).children(content));
        let wrapper = sidebar_tree::item(theme, self.state)
            .child(self.guides(theme, anchors))
            .child(button);
        anchors.root(ID_ITEM.into(), wrapper)
    }

    /// The `<button>`: `TreeRow`'s classes as the scoped stylesheet leaves them.
    ///
    /// The font family is named explicitly rather than inherited, because gpui
    /// can only report the *declared* family and an inherited `.SystemUIFont` is
    /// a string the DOM will never produce.
    fn button(&self, theme: &Theme) -> Div {
        let family = theme.font_sans.primary().unwrap_or("sans-serif");
        div()
            .flex()
            .items_center()
            .justify_start()
            .w_full()
            .min_w_full()
            .h(ROW_HEIGHT)
            .gap_1p5()
            .py_1()
            .pr_1p5()
            .pl(leading_padding(self.depth))
            .border(BUTTON_BORDER)
            .border_color(button_border_color(theme, self.state))
            // `border-radius: 2px !important` from the container-scoped rule,
            // which beats `rounded-md`'s 8. The same 2px the wrapper rounds by,
            // and the same constant.
            .rounded(ITEM_RADIUS)
            .whitespace_nowrap()
            .font_family(family)
            .font_weight(FontWeight::NORMAL)
            // `text-sm` — Tailwind's stock `--text-sm`, 0.875rem. `--ui-text-base`
            // is the same 0.875rem and is the token this design system carries;
            // there is no token for Tailwind's own scale, and inventing one would
            // put a value in the system the system does not have.
            .text_size(theme.ui_text_base.value())
            .line_height(relative(LINE_HEIGHT))
            .text_color(theme.foreground)
    }

    /// `.file-tree-guides` — an `absolute inset-0` layer holding one guide per
    /// level. Empty at depth 0, exactly as the React `guideLevels` array is.
    fn guides(&self, theme: &Theme, anchors: &dyn AnchorSink) -> Div {
        let offset = theme.file_tree_guide_icon_offset.value();
        let guides = (0..self.depth).map(|level| {
            let inset: GuideInset = guide_inset(level, self.previous_depth, self.next_depth);
            anchors.boxed(
                guide_id(level).into(),
                sidebar_tree::guide_at(theme, guide_left(level, offset), inset),
            )
        });
        div().absolute().inset_0().children(guides)
    }

    /// The 14px icon box.
    ///
    /// **Empty**, for the reason [`super::GitStatusRow`]'s is: the React icon is
    /// an SVG the icon-theme registry picks from the file name, there is no
    /// native equivalent, and drawing a substitute would put a shape on screen
    /// for the oracle to converge on. The box is what the contract measures.
    fn icon(theme: &Theme) -> Div {
        div()
            .flex()
            .items_center()
            .flex_shrink_0()
            .w(ICON_SIZE)
            .h(ICON_SIZE)
            .text_color(theme.muted_foreground)
    }

    /// `<span class="relative z-1 flex min-w-0 items-baseline gap-1.5">` and the
    /// name span inside it.
    ///
    /// The wrapper is kept rather than flattened away: it is the box that
    /// carries `min-w-0`, and it is what lets the name shrink while the icon
    /// does not. Anchoring the run inside the span — rather than wrapping the
    /// span in a box anchor — is the same arrangement the git row uses: taffy
    /// stretches a block-level in-flow child to its container's inner width, so
    /// the run's box **is** the span's box and one text anchor reports both.
    fn label(&self, theme: &Theme, anchors: &dyn AnchorSink) -> Div {
        div()
            .flex()
            .items_baseline()
            .gap_1p5()
            .min_w_0()
            .child(
                div().min_w_0().truncate().child(
                    anchors.text(
                        AnchorId::new(ID_NAME).content_sized().line_sized(),
                        self.name.clone(),
                    ),
                ),
            )
            .text_color(theme.foreground)
    }
}

#[cfg(test)]
mod tests {
    use super::{
        AnchorId, BUTTON_BORDER, CONTENT_SIZED, FileTreeRow, ID_BUTTON, ID_ICON, ID_ITEM, ID_NAME,
        INDENT_SIZE, LINE_HEIGHT, LINE_SIZED, button_border_color, guide_id, guide_left,
        leading_padding,
    };
    use crate::components::{ContentLength, RowState};
    use crate::theme::{Color, Theme};
    use gpui::px;

    /// **The indent step is the file explorer's own, not the sidebar tree's.**
    /// They differ by two pixels a level, and the two constants sit next to each
    /// other in this crate, so this is the assertion that keeps one from being
    /// swapped for the other.
    #[test]
    fn the_indent_step_is_sixteen_not_the_sidebar_trees_fourteen() {
        assert_eq!(INDENT_SIZE, px(16.0));
        assert_ne!(INDENT_SIZE, crate::components::INDENT_SIZE);

        assert_eq!(leading_padding(0), px(10.0));
        assert_eq!(leading_padding(1), px(26.0));
        assert_eq!(leading_padding(2), px(42.0));
    }

    /// `left: calc(10px + level × 16px + var(--file-tree-guide-icon-offset))`
    /// with `transform: translateX(-3px)` folded in.
    #[test]
    fn a_guide_sits_at_the_icons_centre_with_the_translate_folded_in() {
        let offset = Theme::DARK.file_tree_guide_icon_offset.value();

        assert_eq!(offset, px(7.0));
        assert_eq!(guide_left(0, offset), px(14.0));
        assert_eq!(guide_left(1, offset), px(30.0));
        assert_eq!(guide_left(2, offset), px(46.0));
    }

    #[test]
    fn guide_ids_are_zero_based_and_namespaced_to_this_surface() {
        assert_eq!(guide_id(0), "file-row-guide-0");
        assert_eq!(guide_id(3), "file-row-guide-3");
    }

    /// The one state the git row cannot reach. `focused` is the *only* thing
    /// that moves the border, and it moves it off `transparent` — which is a
    /// comparable delta because the border is 1px wide in every state and
    /// `ANCHORS.md` v1.3 compares `border.color` wherever `w > 0`.
    #[test]
    fn focus_is_the_only_state_that_moves_the_buttons_border() {
        for theme in [Theme::LIGHT, Theme::DARK] {
            assert_eq!(
                button_border_color(&theme, RowState::resting()),
                Color::TRANSPARENT,
            );
            for state in [
                RowState {
                    hovered: true,
                    ..RowState::resting()
                },
                RowState {
                    selected: true,
                    ..RowState::resting()
                },
            ] {
                assert_eq!(button_border_color(&theme, state), Color::TRANSPARENT);
            }

            let focused = button_border_color(
                &theme,
                RowState {
                    focused: true,
                    ..RowState::resting()
                },
            );
            assert_eq!(focused, theme.accent.mix(42.0, theme.border));
            assert_ne!(focused, Color::TRANSPARENT);
        }
        assert_eq!(BUTTON_BORDER, px(1.0));
    }

    /// The three backgrounds are the wrapper's, and they are the sidebar tree's
    /// — the `::before` group is *unscoped*, so both surfaces get it. This
    /// asserts the reuse rather than restating the colours, because a second
    /// spelling of them is a second thing to drift.
    #[test]
    fn the_wrapper_background_is_the_shared_unscoped_rule() {
        let theme = Theme::DARK;

        assert_eq!(
            crate::components::item_background(&theme, RowState::resting()),
            None,
        );
        assert_eq!(
            crate::components::item_background(
                &theme,
                RowState {
                    selected: true,
                    ..RowState::resting()
                }
            ),
            Some(theme.accent),
        );
    }

    /// The fixture shares the git row's strings, so the same content length
    /// means the same name on both surfaces.
    #[test]
    fn the_fixture_shares_the_git_rows_names() {
        assert_eq!(FileTreeRow::fixture(ContentLength::Short).name, "a.ts");
        assert_eq!(
            FileTreeRow::fixture(ContentLength::Normal).name,
            "resolve-terminal-connection.ts",
        );
        assert_eq!(
            FileTreeRow::fixture(ContentLength::Overflow).name,
            "an-extremely-long-file-name-that-must-truncate-in-the-sidebar-row.ts",
        );
        assert_eq!(FileTreeRow::fixture(ContentLength::Normal).state, RowState::resting());
    }

    /// v1.5 and v1.6, both on the name and on nothing else.
    ///
    /// The button is the trap here: it paints text — it is where the font is
    /// declared — but `h-6` authors its height at 24px around a 20px line box,
    /// so declaring it would invent a 4px delta on an anchor both engines agree
    /// on. The same reasoning that keeps the git row's badge off its list.
    #[test]
    fn only_the_name_is_declared_and_the_button_is_not() {
        assert_eq!(CONTENT_SIZED, [ID_NAME]);
        assert_eq!(LINE_SIZED, [ID_NAME]);

        for id in [ID_ITEM, ID_BUTTON, ID_ICON] {
            assert!(!CONTENT_SIZED.contains(&id), "{id}");
            assert!(!LINE_SIZED.contains(&id), "{id}");
        }
        assert!(!LINE_SIZED.contains(&"file-row-guide-0"));

        let declared = AnchorId::new(ID_NAME).content_sized().line_sized();
        assert!(declared.content_sized && declared.line_sized);
    }

    /// `text-sm`'s line height, spelled as the two numbers the stylesheet
    /// divides — 14px text on a 20px line box, where the git row is 14 on 18.9.
    #[test]
    fn the_line_height_is_text_sms_and_lands_on_twenty_pixels() {
        assert!((LINE_HEIGHT - 1.25 / 0.875).abs() < f32::EPSILON);
        assert!((LINE_HEIGHT * 14.0 - 20.0).abs() < 0.001, "{LINE_HEIGHT}");
        // And it is *not* the git row's authored `leading-[1.35]`.
        assert!((LINE_HEIGHT - 1.35).abs() > 0.05);
    }
}
