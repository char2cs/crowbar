//! The sidebar tree row's chrome: the three-layer background, the indent
//! arithmetic and the indent guides.
//!
//! This is the native half of `web/src/components/ui/sidebar-tree.tsx` +
//! `web/src/components/ui/tree-row.tsx`, **as those two are actually rendered**
//! — which is not what their Tailwind class lists say. The class lists are
//! overridden by `web/src/features/file-explorer/styles/file-explorer-tree.css`,
//! and a faithful transcription of the classes produces a row that is wrong and
//! looks right. What the cascade really resolves to is recorded here, one note
//! per surprise, because the next reader will otherwise "fix" it back.
//!
//! # The three layers
//!
//! | Layer | What it is | Paints |
//! |---|---|---|
//! | `.file-tree-item` | full-width wrapper | **every visible row background**, via its `::before` |
//! | `.file-tree-row` `<button>` | the row proper | nothing — `bg-transparent`, and `.file-tree-row:hover` is pinned transparent too |
//! | content | icon, spans, badges | text |
//!
//! The middle layer is not where the background lives, and the pseudo-element
//! that *is* has no DOM node — see `native/oracle/ANCHORS.md` §3. gpui has no
//! pseudo-elements, so the wrapper paints the background directly; because the
//! rule is `position: absolute; inset: 0` on a wrapper with no padding and no
//! border, the two are the same box and the same paint.
//!
//! # What the cascade actually resolves to
//!
//! * `hover:bg-muted` on the button is **dead** — `.file-tree-row:hover` pins
//!   the button's background transparent, unscoped, with `!important`.
//! * `active && bg-accent/20` on the button is **dead** — `SidebarTreeRow`
//!   reads its `active` prop for `data-active` on the wrapper and never passes
//!   it down, so `TreeRow`'s `active` is always `false`.
//! * `border-none` and `border` are different tailwind-merge groups, so both
//!   survive: `border-style: none` with `border-width: 1px`. CSS computes a
//!   `none`-styled border's width to **zero**, so the button has **no border**.
//! * `min-w-max` (from `TreeRow`) loses to `min-w-0` (from the call site) —
//!   same tailwind-merge group, later wins.
//! * `rounded-md` survives, and this project redefines `--radius-md` as
//!   `calc(var(--radius) * 0.8)` = **8px**, not Tailwind's stock 6.
//!
//! Three `!important` rules that *do* fire are unscoped and are reproduced:
//! `.file-tree-item { width: 100% }`, the `::before` group, and
//! `.file-tree-row:hover { background: transparent }`. The rest of that
//! stylesheet is scoped to `.file-tree-container`, which the git status panel
//! is **not** inside — measured, not deduced.

use gpui::{Div, ParentElement as _, Pixels, Styled as _, div, px};

use crate::theme::{Color, Theme};

/// `SIDEBAR_TREE_BASE_INDENT` — the row's leading padding at depth 0.
pub const BASE_INDENT: Pixels = px(10.0);

/// `SIDEBAR_TREE_INDENT_SIZE` — one nesting level's worth of leading padding.
pub const INDENT_SIZE: Pixels = px(14.0);

/// `SIDEBAR_TREE_ICON_SIZE` — the file icon's box, both axes.
pub const ICON_SIZE: Pixels = px(14.0);

/// `h-6`: the row's border-box height.
pub const ROW_HEIGHT: Pixels = px(24.0);

/// `.file-tree-item::before { border-radius: 2px }`.
///
/// A literal, and the only one here that no token carries: the file-tree
/// stylesheet writes `2px` inline rather than reaching for `--radius-*`, and the
/// nearest token, `--radius-sm`, is 6px. Inventing a token for it would put a
/// value in the design system that the design system does not have.
pub const ITEM_RADIUS: Pixels = px(2.0);

/// `.file-tree-guide { width: 7px }` — the guide's hit target, not its rule.
pub const GUIDE_WIDTH: Pixels = px(7.0);

/// `.file-tree-guide::before { left: 3px; width: 1px }` — the visible rule.
pub const GUIDE_RULE_INSET: Pixels = px(3.0);

/// The rule itself.
pub const GUIDE_RULE_WIDTH: Pixels = px(1.0);

/// `.file-tree-guide { transform: translateX(-3px) }`.
///
/// gpui has no transform on a `div`, so the shift is folded into the guide's
/// `left`. That is exact for a translation, and it is the only way to express
/// it — see the crate's report for the general case.
pub const GUIDE_SHIFT: Pixels = px(3.0);

/// How far a guide is pulled in at the end of a run, so consecutive runs do not
/// butt against each other.
pub const GUIDE_END_INSET: Pixels = px(4.0);

/// `opacity: 0.9` on `.file-tree-guide`, expressed as an alpha scale.
///
/// The contract has no opacity field and gpui's `opacity` is a subtree
/// property, so the guide's rule is mixed to the same result instead:
/// compositing a single solid fill at 0.9 is exactly that fill with 0.9× the
/// alpha.
pub const GUIDE_OPACITY_PERCENT: f32 = 90.0;

/// The visual state the row is painted in.
///
/// **A parameter, not a gpui `.hover(…)` refinement.** gpui applies hover and
/// active styling from runtime interaction state that only materialises once a
/// hitbox has been hit, so the extractor — which reads an element's *base*
/// style during `prepaint` — would report the resting paint for every hovered
/// cell of the matrix and converge while proving nothing.
///
/// It is also closer to the original: in the React app these backgrounds are
/// painted by `.file-tree-item::before` keyed off the **wrapper's** `:hover`
/// and `data-active`, not off the button's own interaction state.
#[derive(Clone, Copy, Debug, Default, PartialEq, Eq)]
pub struct RowState {
    /// The pointer is over the row.
    pub hovered: bool,
    /// The row holds keyboard focus.
    pub focused: bool,
    /// `data-active='true'` — the row is the selected one.
    pub selected: bool,
}

impl RowState {
    /// The resting row.
    #[must_use]
    pub const fn resting() -> Self {
        Self {
            hovered: false,
            focused: false,
            selected: false,
        }
    }
}

/// A guide's vertical insets within the row.
#[derive(Clone, Copy, Debug, PartialEq)]
pub struct GuideInset {
    /// Distance from the top of the row.
    pub top: Pixels,
    /// Distance from the bottom of the row.
    pub bottom: Pixels,
}

/// The row's leading padding at `depth`.
///
/// Padding rather than a spacer element, which is what lets the background span
/// the full width — see `TreeRow`'s own comment.
#[must_use]
pub fn leading_padding(depth: u16) -> Pixels {
    BASE_INDENT + INDENT_SIZE * f32::from(depth)
}

/// Where a guide's 7px host box starts, with `translateX(-3px)` folded in.
///
/// `icon_offset` is `--file-tree-guide-icon-offset`, which is a theme token
/// rather than a constant because the stylesheet declares it as one.
#[must_use]
pub fn guide_left(level: u16, icon_offset: Pixels) -> Pixels {
    BASE_INDENT + INDENT_SIZE * f32::from(level) + icon_offset - GUIDE_SHIFT
}

/// A guide is inset at the top when the row *begins* a run at that level, and
/// at the bottom when it *ends* one.
///
/// "Begins" is `previousDepth <= level`: the row above is at or shallower than
/// this level, so no guide at this level continues into it.
#[must_use]
pub fn guide_inset(level: u16, previous_depth: u16, next_depth: u16) -> GuideInset {
    GuideInset {
        top: if previous_depth <= level {
            GUIDE_END_INSET
        } else {
            px(0.0)
        },
        bottom: if next_depth <= level {
            GUIDE_END_INSET
        } else {
            px(0.0)
        },
    }
}

/// The wrapper's background — the one `.file-tree-item::before` paints.
///
/// `None` is `background-color: transparent`, which is what the resting and
/// focused rows have. It is returned rather than painted so that the element
/// carries no background at all: the extractor reports an unpainted box and a
/// `transparent` one identically, and not painting is the truer statement.
#[must_use]
pub fn item_background(theme: &Theme, state: RowState) -> Option<Color> {
    if state.selected {
        // `.file-tree-item[data-active='true']::before { background: var(--accent) }`
        Some(theme.accent)
    } else if state.hovered {
        // `--file-tree-hover-bg: color-mix(in srgb, var(--accent) 68%, transparent)`
        Some(theme.file_tree_hover_bg)
    } else {
        None
    }
}

/// The `.file-tree-item` wrapper: full width, and the layer that paints.
#[must_use]
pub fn item(theme: &Theme, state: RowState) -> Div {
    let wrapper = div().relative().w_full().min_w_full().rounded(ITEM_RADIUS);
    match item_background(theme, state) {
        Some(background) => wrapper.bg(background),
        None => wrapper,
    }
}

/// The `<button>`: `flex w-full min-w-0 h-6 gap-1.5 px-1.5 py-1 rounded-md`,
/// with the leading padding overridden inline by the depth.
///
/// It paints **no background in any state** and has **no border** — see the
/// module docs for why both of those survive a class list that appears to say
/// otherwise. `focused` therefore changes nothing here; the `:focus-visible`
/// border rule that would have is scoped to `.file-tree-container`, which this
/// surface is not inside.
#[must_use]
pub fn row_button(theme: &Theme, depth: u16) -> Div {
    div()
        .flex()
        .items_center()
        .justify_start()
        .w_full()
        .min_w_0()
        .h(ROW_HEIGHT)
        .gap_1p5()
        .py_1()
        .pr_1p5()
        .pl(leading_padding(depth))
        .rounded(theme.radius_md.value())
        .whitespace_nowrap()
        .text_color(theme.foreground)
}

/// One indent guide: a 7px transparent host holding the 1px rule that is
/// actually visible.
///
/// The host is what carries the anchor, because the host is what the DOM
/// tags — its `::before` is not `inset: 0` and so cannot be read through the
/// pseudo-element shortcut in `native/oracle/ANCHORS.md` §3.
#[must_use]
pub fn guide(theme: &Theme, level: u16, previous_depth: u16, next_depth: u16) -> Div {
    let inset = guide_inset(level, previous_depth, next_depth);
    div()
        .absolute()
        .left(guide_left(level, theme.file_tree_guide_icon_offset.value()))
        .top(inset.top)
        .bottom(inset.bottom)
        .w(GUIDE_WIDTH)
        .child(
            div()
                .absolute()
                .left(GUIDE_RULE_INSET)
                .top_0()
                .bottom_0()
                .w(GUIDE_RULE_WIDTH)
                .bg(theme
                    .tree_guide_color
                    .mix(GUIDE_OPACITY_PERCENT, Color::TRANSPARENT)),
        )
}

#[cfg(test)]
mod tests {
    use super::{
        BASE_INDENT, GUIDE_END_INSET, INDENT_SIZE, RowState, guide_inset, guide_left,
        item_background, leading_padding,
    };
    use crate::theme::Theme;
    use gpui::px;

    #[test]
    fn the_leading_padding_is_ten_plus_fourteen_a_level() {
        assert_eq!(leading_padding(0), px(10.0));
        assert_eq!(leading_padding(1), px(24.0));
        assert_eq!(leading_padding(2), px(38.0));
        assert_eq!(leading_padding(7), px(108.0));
    }

    /// Spelled as the arithmetic rather than as more literals, so a change to
    /// either constant cannot pass by leaving the literals alone.
    #[test]
    fn the_leading_padding_is_the_constants_and_nothing_else() {
        for depth in 0..12_u16 {
            assert_eq!(
                leading_padding(depth),
                BASE_INDENT + INDENT_SIZE * f32::from(depth),
            );
        }
    }

    /// `left: calc(baseIndent + level * 14px + var(--file-tree-guide-icon-offset))`
    /// with `transform: translateX(-3px)` folded in: 10 + 14n + 7 - 3.
    #[test]
    fn a_guide_sits_at_the_icons_centre_with_the_translate_folded_in() {
        let offset = px(7.0);
        assert_eq!(guide_left(0, offset), px(14.0));
        assert_eq!(guide_left(1, offset), px(28.0));
        assert_eq!(guide_left(2, offset), px(42.0));
    }

    /// The offset is a token, so a theme that moved it moves the guides.
    #[test]
    fn the_guide_offset_comes_from_the_token_not_a_constant() {
        assert_eq!(
            guide_left(0, Theme::DARK.file_tree_guide_icon_offset.value()),
            px(14.0),
        );
        assert_eq!(guide_left(0, px(11.0)), px(18.0));
    }

    /// A row in the middle of a run at every level: no guide is capped.
    #[test]
    fn a_guide_that_runs_through_the_row_is_not_inset() {
        let inset = guide_inset(0, 3, 3);
        assert_eq!(inset.top, px(0.0));
        assert_eq!(inset.bottom, px(0.0));
    }

    /// The row above is shallower than this level, so the run starts here.
    #[test]
    fn a_guide_that_begins_at_this_row_is_inset_at_the_top() {
        let inset = guide_inset(2, 2, 3);
        assert_eq!(inset.top, GUIDE_END_INSET);
        assert_eq!(inset.bottom, px(0.0));
    }

    #[test]
    fn a_guide_that_ends_at_this_row_is_inset_at_the_bottom() {
        let inset = guide_inset(2, 3, 1);
        assert_eq!(inset.top, px(0.0));
        assert_eq!(inset.bottom, GUIDE_END_INSET);
    }

    /// A row whose neighbours default to its **own** depth caps *nothing*, and
    /// this is worth pinning because it reads the other way round at a glance.
    ///
    /// The defaults are `previousDepth = nextDepth = depth`, and the levels a
    /// row draws are `0..depth`, so `previousDepth <= level` is false at every
    /// one of them. A single row rendered on its own therefore has full-height
    /// guides, not capped ones — which is exactly what the gate surface shows,
    /// and a test that asserted otherwise would have sent someone hunting a
    /// layout bug that is not there.
    #[test]
    fn a_row_whose_neighbours_are_at_its_own_depth_caps_nothing() {
        for depth in 1..4_u16 {
            for level in 0..depth {
                let inset = guide_inset(level, depth, depth);
                assert_eq!(inset.top, px(0.0), "depth {depth}, level {level}");
                assert_eq!(inset.bottom, px(0.0), "depth {depth}, level {level}");
            }
        }
    }

    /// The cap needs a genuinely shallower neighbour: a row that follows a
    /// root-level row begins a run at every level it draws.
    #[test]
    fn a_row_after_a_shallower_one_caps_every_guide_at_the_top() {
        for level in 0..3_u16 {
            let inset = guide_inset(level, 0, 5);
            assert_eq!(inset.top, GUIDE_END_INSET);
            assert_eq!(inset.bottom, px(0.0));
        }
    }

    /// The three backgrounds of `.file-tree-item::before`, and the fact that
    /// focus is not one of them.
    #[test]
    fn the_wrapper_paints_transparent_hover_and_accent_and_nothing_else() {
        let theme = Theme::DARK;

        assert_eq!(item_background(&theme, RowState::resting()), None);
        assert_eq!(
            item_background(
                &theme,
                RowState {
                    hovered: true,
                    ..RowState::resting()
                }
            ),
            Some(theme.file_tree_hover_bg),
        );
        assert_eq!(
            item_background(
                &theme,
                RowState {
                    selected: true,
                    ..RowState::resting()
                }
            ),
            Some(theme.accent),
        );
        assert_eq!(
            item_background(
                &theme,
                RowState {
                    focused: true,
                    ..RowState::resting()
                }
            ),
            None,
        );
    }

    /// `data-active` wins over `:hover`: the attribute selector and the
    /// pseudo-class have the same specificity, and the active rule is written
    /// second.
    #[test]
    fn selected_beats_hovered() {
        let theme = Theme::LIGHT;
        let both = RowState {
            hovered: true,
            focused: true,
            selected: true,
        };

        assert_eq!(item_background(&theme, both), Some(theme.accent));
    }

    /// The hover background is a token, not a mix done at the call site — but
    /// the two have to be the same colour, or a row drifts from the stylesheet.
    #[test]
    fn the_hover_background_is_the_accent_at_sixty_eight_percent() {
        for theme in [Theme::LIGHT, Theme::DARK] {
            assert_eq!(
                theme.file_tree_hover_bg,
                theme.accent.mix(68.0, crate::theme::Color::TRANSPARENT),
            );
        }
    }
}
