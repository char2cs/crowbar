//! `search_toggle_icons` — four icons behind one anchor id, three of which the
//! contract can only see the box of and one of which is a **text run**.
//!
//! The native half of `web/src/components/ui/search-toggle-icons.tsx`, which is
//! not a component at all but a `Record` of four `ReactNode`s: three
//! `@phosphor-icons/react` glyphs and one `<span>` reading `Aa`. See
//! `native/mapping/search-toggle-icons.md`.
//!
//! # One id, four renderings — `ANCHORS.md` v1.8
//!
//! All four carry `data-oracle-id="search-toggle-icon"`, exactly as the four
//! live `<Kbd>`s all carry `kbd`. They are siblings in the same options row, so
//! four *distinct* ids would name a set no single root contains and no snapshot
//! could hold; one id plus `extractSnapshot`'s `index` gives one anchor rooted
//! at the glyph itself, which is the only arrangement that compares.
//!
//! # The three glyphs: the P3.2 trap in its purest form
//!
//! `<CaseSensitive />` is passed with **no props at all**, so phosphor emits
//! `width="1em" height="1em"` — 14px against the button's `sm:text-sm`. It
//! renders at **16**, because presentational attributes have no specificity and
//! `button`'s `sm:[&_svg:not([class*='size-'])]:size-4` beats them. Measured
//! live: `16 × 16` with `class` absent.
//!
//! So the glyphs' box is [`button::Size::Default::icon`] and belongs to the
//! *host button*, not to this file — which is the mirror image of
//! [`sidebar_toggle_icon`](super::sidebar_toggle_icon), where the primitive
//! pins `size-4` precisely to escape the same rule. Two icon files in the same
//! directory, opposite answers, and the reason is one class.
//!
//! `button`'s `[&_svg]:-mx-0.5` does not reach the anchor: a margin is outside
//! the border box, so the reference's `bounds.w` is the 16 and not the 12
//! [`button::Size::glyph_box`] folds it into. The taffy defect that constant
//! documents cannot bite here either — the toggle button pins `size-6`, so its
//! main size is definite.
//!
//! **What the contract sees on a glyph cell is its box and nothing else.** No
//! `fg`, no `font`, no `text`: an `<svg>`'s children are elements, and
//! `oracleOwnText` looks for text nodes. In particular the toggle's
//! **active state is invisible on these three** — `searchToggleButtonVariants`
//! moves `color` from `--muted-foreground` to `--foreground`, and the glyph
//! resolves it through `fill: currentColor`, which has no field.
//!
//! # `preserveCase` is the interesting one: a real run, and both declarations
//!
//! `<span className="ui-font ui-text-xs font-semibold">Aa</span>`, blockified by
//! the toggle button's flex — measured `display: block`, `badge`'s and `kbd`'s
//! reason a third time. It carries:
//!
//! * `content_sized` — no authored width, so the box is the run's max-content
//!   width. Reference `14.48`, `text_width` `14.48`.
//! * `line_sized` — no authored height either, so the box **is** the line box.
//!   And here that declaration is *not* numerically free: `ui-text-xs` sets a
//!   `font-size` and **no line-height**, so the ratio is inherited from the
//!   button's `sm:text-sm` and lands on `11 × 1.25/0.875 = 15.714`, which `WebKit`
//!   floors to a `bounds.h` of **15**. Without the declaration the port's
//!   `pixel_snap(15.714) = 15.5` would be compared against 15 — exactly on the
//!   ±0.5 boundary. With it, 15.5 against 15.714 is 0.214.
//!
//! It is also the **only** cell on this surface where the active state is
//! visible: `fg` moves from `#a4a4a4ff` to `#f5f5f5ff`, and `fg` is a compared
//! field.

use gpui::{AnyElement, Div, FontWeight, Pixels, SharedString, Styled as _, div, px, relative};

use crate::anchor::{AnchorId, AnchorSink};
use crate::primitives::button;
use crate::surfaces::rows::git_status_row::Breakpoint;
use crate::theme::{Theme, ui_sans_font};

/// The single anchor id, carried by all four renderings.
pub const ID_SEARCH_TOGGLE_ICON: &str = "search-toggle-icon";

/// Declared **only on the `preserve-case` cell**, which is the one rendering
/// with a run. The three glyph cells take an authored box off the host button
/// and declare nothing — see the module docs, and `label`'s precedent for a
/// list that names an anchor the render path may still drop.
pub const CONTENT_SIZED: [&str; 1] = [ID_SEARCH_TOGGLE_ICON];

/// Declared **only on the `preserve-case` cell**, and only while it paints its
/// legend: `ANCHORS.md` v1.6 makes the declaration valid solely on an anchor
/// carrying a `font`, so §8.3's `empty` drops it.
pub const LINE_SIZED: [&str; 1] = [ID_SEARCH_TOGGLE_ICON];

/// `font-semibold` — `--font-weight-semibold: 600`, which the reference reports.
pub const WEIGHT: FontWeight = FontWeight::SEMIBOLD;

/// The legend `preserveCase` paints. Hard-coded in the primitive, so no call
/// site can change it and `--content` cannot move it.
pub const LEGEND: &str = "Aa";

/// Which of the four the cell renders.
#[derive(Clone, Copy, Debug, PartialEq, Eq)]
pub enum Toggle {
    /// `TextAa` — "Match case". A phosphor glyph. Live in **both** importers.
    CaseSensitive,
    /// `TextT` — "Match whole word". A phosphor glyph. Live in both.
    WholeWord,
    /// `BracketsCurly` — "Use regular expression". A phosphor glyph. Live in
    /// both.
    Regex,
    /// The `Aa` span — "Preserve case". **`find-bar.tsx` only**, and only while
    /// the replace row is expanded; `terminal-search.tsx` does not offer it.
    PreserveCase,
}

/// Every toggle, for `--help` and the closed-vocabulary test.
pub const ALL_TOGGLES: [Toggle; 4] = [
    Toggle::CaseSensitive,
    Toggle::WholeWord,
    Toggle::Regex,
    Toggle::PreserveCase,
];

impl Toggle {
    /// The word `--toggle` takes.
    #[must_use]
    pub fn name(self) -> &'static str {
        match self {
            Self::CaseSensitive => "case-sensitive",
            Self::WholeWord => "whole-word",
            Self::Regex => "regex",
            Self::PreserveCase => "preserve-case",
        }
    }

    /// The string this toggle paints, or `None` where it is an `<svg>`.
    ///
    /// This is the whole difference between the two shapes: a legend makes the
    /// anchor a box *and* a run, and its absence makes it a box the contract can
    /// say nothing else about.
    #[must_use]
    pub fn legend(self) -> Option<SharedString> {
        match self {
            Self::CaseSensitive | Self::WholeWord | Self::Regex => None,
            Self::PreserveCase => Some(SharedString::new_static(LEGEND)),
        }
    }

    /// Whether both live importers render this toggle.
    ///
    /// **`preserve-case` is the one that does not**: `terminal-search.tsx`
    /// passes three options, `find-bar.tsx` four — and its fourth only while the
    /// replace row is open. Recorded because it is the reachability fact behind
    /// the capture: driving the reference for the run-shaped cell needs the
    /// editor's find bar *with replace expanded*, which the terminal's search
    /// can never produce.
    #[must_use]
    pub fn in_every_importer(self) -> bool {
        !matches!(self, Self::PreserveCase)
    }
}

/// The glyph box the host button's icon rule gives an `<svg>` carrying no
/// `size-` class.
///
/// `button::Size::Default` because `SearchPopover` passes no `size` prop — 18px
/// below the `sm` breakpoint and 16 at or above it. Read off `button`'s own
/// table rather than restated, so the two cannot drift.
#[must_use]
pub fn glyph_extent(breakpoint: Breakpoint) -> Pixels {
    button::Size::Default.icon(breakpoint)
}

/// The unitless line-height the `Aa` span **inherits** from its button.
///
/// `ui-text-xs` sets a `font-size` and nothing else, so the ratio comes from the
/// button's own `text-base` / `sm:text-sm` step: `1.5` below the breakpoint and
/// `1.25/0.875` at or above it. Taken from `button`'s table for the same reason
/// [`glyph_extent`] is.
#[must_use]
pub fn inherited_line_height(theme: &Theme, breakpoint: Breakpoint) -> f32 {
    button::Size::Default
        .type_step(theme, breakpoint)
        .line_height
}

/// One entry of `SEARCH_TOGGLE_ICONS`, as rendered inside its toggle button.
#[derive(Clone, Copy, Debug, PartialEq)]
pub struct SearchToggleIcon {
    /// Which of the four.
    pub toggle: Toggle,
    /// Which side of `sm:` the viewport is on. Real on **both** shapes: it moves
    /// the glyph's box 18 → 16 and the span's inherited line-height 1.5 → 1.43.
    pub breakpoint: Breakpoint,
    /// `searchToggleButtonVariants({ active: true })` — the toggle is on.
    ///
    /// Visible to the contract **only on `preserve-case`**, where it moves the
    /// run's `fg`. On the three glyph cells it moves `fill: currentColor`, which
    /// has no field.
    pub active: bool,
    /// §8.3's `empty`: the icon with its box merged away to zero.
    ///
    /// Expressible from a call site — the toggle button renders whatever
    /// `SEARCH_TOGGLE_ICONS` holds, and a `size-0` on either shape wins the same
    /// way the button's own `size-4` rule does — and taken by none. Both engines
    /// give a zero box zero area and report `visible: false`.
    ///
    /// It does **not** remove the legend. The run stays painted and stays
    /// recorded; what changes is that the box is now *authored* on both axes,
    /// so the anchor drops `content_sized` and `line_sized` — declarations that
    /// would otherwise claim a zero box measured itself.
    pub empty: bool,
}

impl SearchToggleIcon {
    /// The captured cell: the editor find bar's **Preserve case** toggle at a
    /// 1714px viewport, replace expanded, resting — `14.48 × 15`,
    /// `fg #a4a4a4ff`, `CalSansUI` 11/15.71 at weight 600, `content_sized` and
    /// `line_sized`.
    ///
    /// The run-shaped cell rather than one of the three glyphs, for `kbd`'s
    /// reason: it is the only one the contract can see more than a box of.
    /// `/tmp/p3-ref-search-toggle-icons-glyph.json` archives a glyph cell
    /// beside it.
    #[must_use]
    pub fn fixture() -> Self {
        Self {
            toggle: Toggle::PreserveCase,
            breakpoint: Breakpoint::Sm,
            active: false,
            empty: false,
        }
    }

    /// The legend this cell paints, or `None` where this toggle is an `<svg>`.
    ///
    /// `empty` does not remove it — see the field.
    #[must_use]
    pub fn legend(self) -> Option<SharedString> {
        self.toggle.legend()
    }

    /// The colour the run takes — `text-foreground` when the toggle is on,
    /// `text-muted-foreground` when it is off, both inherited from the button.
    #[must_use]
    pub fn foreground(self, theme: &Theme) -> crate::theme::Color {
        if self.active {
            theme.color_foreground
        } else {
            theme.color_muted_foreground
        }
    }

    /// The box, for whichever of the two shapes this cell is.
    ///
    /// No background, no radius and no border on either: `search-toggle-icons.tsx`
    /// names none and preflight zeroes `border` for every element. The reference
    /// agrees on both shapes.
    fn shell(self, theme: &Theme) -> Div {
        if self.legend().is_some() {
            // The `Aa` span. `ui-font` resolves to the same `CalSansUI` the
            // button inherits, and is named rather than inherited for
            // `git_status_row`'s reason: `ANCHORS.md` v1.2 makes `font.family`
            // the *declared* first family, and an inherited style reports
            // macOS's `.SystemUIFont`, which the DOM never produces.
            let mut element = div()
                .font(ui_sans_font(theme))
                .text_size(theme.ui_text_xs.value())
                .line_height(relative(inherited_line_height(theme, self.breakpoint)))
                .font_weight(WEIGHT)
                .text_color(self.foreground(theme));
            if self.empty {
                element = element.w(px(0.0)).h(px(0.0));
            }
            return element;
        }

        // A phosphor `<svg>`, as an empty box: there is no native equivalent,
        // and drawing a substitute would put a shape on screen for the oracle
        // to converge on.
        let extent = if self.empty {
            px(0.0)
        } else {
            glyph_extent(self.breakpoint)
        };
        div().flex_shrink_0().w(extent).h(extent)
    }

    /// The element, with its one anchor.
    ///
    /// The declarations ride on the run and not on the glyph: `content_sized`
    /// wherever there is a legend and the box is the run's, and `line_sized`
    /// with it — an anchor that declared it without a `font` is a refusal rather
    /// than a delta. §8.3's `empty` authors both axes, so it drops both.
    pub fn render(self, theme: &Theme, anchors: &dyn AnchorSink) -> AnyElement {
        let shell = self.shell(theme);
        match self.legend() {
            Some(legend) => {
                let mut id = AnchorId::from(ID_SEARCH_TOGGLE_ICON);
                if !self.empty {
                    id = id.content_sized().line_sized();
                }
                anchors.boxed_text(id, shell, legend)
            }
            None => anchors.boxed(AnchorId::from(ID_SEARCH_TOGGLE_ICON), shell),
        }
    }
}

#[cfg(test)]
mod tests {
    use super::{
        ALL_TOGGLES, ID_SEARCH_TOGGLE_ICON, LEGEND, SearchToggleIcon, Toggle, WEIGHT, glyph_extent,
        inherited_line_height,
    };
    use crate::surfaces::rows::git_status_row::Breakpoint;
    use crate::theme::{Appearance, Theme};
    use gpui::{FontWeight, px};

    /// Both declarations name the one anchor, and the render path is what
    /// decides per cell — `label`'s arrangement. The glyph cells make neither
    /// claim, which is asserted below rather than left to the constant.
    #[test]
    fn both_declarations_are_made_and_only_by_the_run_shaped_cell() {
        assert_eq!(super::CONTENT_SIZED, [ID_SEARCH_TOGGLE_ICON]);
        assert_eq!(super::LINE_SIZED, [ID_SEARCH_TOGGLE_ICON]);

        assert_eq!(
            SearchToggleIcon::fixture().legend().as_deref(),
            Some(LEGEND)
        );
        for toggle in ALL_TOGGLES {
            let icon = SearchToggleIcon {
                toggle,
                ..SearchToggleIcon::fixture()
            };
            assert_eq!(
                icon.legend().is_some(),
                toggle == Toggle::PreserveCase,
                "{}",
                toggle.name(),
            );
        }
    }

    /// §8.3's `empty` authors a **zero box on both axes** and **keeps the run**
    /// — which is what separates it from `label`'s empty cell, where the call
    /// site supplies the children and can withhold them. Here the legend is the
    /// primitive's own and nothing can take it away.
    ///
    /// That the declarations come off with the measure is decided by the render
    /// path and asserted where there is a window to lay it out in:
    /// `crates/crowbar-app/src/row_layout/search_toggle_icons.rs`.
    #[test]
    fn the_empty_cell_keeps_its_run() {
        let empty = SearchToggleIcon {
            empty: true,
            ..SearchToggleIcon::fixture()
        };
        assert_eq!(empty.legend().as_deref(), Some(LEGEND));
        for toggle in ALL_TOGGLES {
            let cell = SearchToggleIcon { toggle, ..empty };
            assert_eq!(cell.legend(), toggle.legend(), "{}", toggle.name());
        }
    }

    /// **The glyph box is the host button's, and it moves at the breakpoint** —
    /// 18 below `sm`, 16 at or above. This is the trap P3.2 measured, and the
    /// two numbers being different is what makes `--viewport-width` real here.
    #[test]
    fn the_glyph_takes_the_buttons_icon_rule_at_both_breakpoints() {
        assert_eq!(glyph_extent(Breakpoint::Sm), px(16.0));
        assert_eq!(glyph_extent(Breakpoint::Base), px(18.0));
        assert_ne!(glyph_extent(Breakpoint::Sm), glyph_extent(Breakpoint::Base));
    }

    /// **The line-height is inherited, not authored**, and it is what makes the
    /// reference's `bounds.h` 15 against a `font.line_height` of 15.71.
    ///
    /// `ui-text-xs` is 11px, and the button's `sm:text-sm` ratio is `1.25/0.875`
    /// — the product is the reference's number. The `Base` arm is the control:
    /// a ratio that did not move at the breakpoint would make
    /// `--viewport-width` vacuous on this shape, and it is not.
    #[test]
    fn the_line_box_is_the_inherited_ratio_over_the_ui_text_xs_size() {
        for appearance in [Appearance::Light, Appearance::Dark] {
            let theme = Theme::for_appearance(appearance);
            let size = theme.ui_text_xs.value().0 * 16.0;
            assert!((size - 11.0).abs() < 1e-4, "{appearance:?}: {size}");

            let small = inherited_line_height(&theme, Breakpoint::Sm);
            assert!((size * small - 15.714_286).abs() < 1e-3, "{}", size * small);

            let base = inherited_line_height(&theme, Breakpoint::Base);
            assert!((size * base - 16.5).abs() < 1e-3, "{}", size * base);
            assert!(
                (small - base).abs() > 1e-3,
                "the ratio moves at the breakpoint"
            );
        }
    }

    /// The active state moves the run's colour and **only** the run's — the two
    /// tokens are different, so this is not an assertion that would hold either
    /// way.
    #[test]
    fn the_active_toggle_paints_the_foreground_token() {
        for appearance in [Appearance::Light, Appearance::Dark] {
            let theme = Theme::for_appearance(appearance);
            let resting = SearchToggleIcon::fixture();
            let engaged = SearchToggleIcon {
                active: true,
                ..resting
            };
            assert_eq!(resting.foreground(&theme), theme.color_muted_foreground);
            assert_eq!(engaged.foreground(&theme), theme.color_foreground);
            assert_ne!(
                theme.color_muted_foreground, theme.color_foreground,
                "{appearance:?}",
            );
        }
    }

    /// `preserve-case` is the one toggle the terminal's search never renders —
    /// the reachability fact behind which cell was captured.
    #[test]
    fn only_preserve_case_is_missing_from_an_importer() {
        for toggle in ALL_TOGGLES {
            assert_eq!(
                toggle.in_every_importer(),
                toggle != Toggle::PreserveCase,
                "{}",
                toggle.name(),
            );
        }
    }

    /// The vocabulary is closed and its words are unique, the legend is the
    /// primitive's own, and the fixture is the captured cell.
    #[test]
    fn the_toggle_vocabulary_is_closed() {
        let mut names: Vec<_> = ALL_TOGGLES.iter().map(|t| t.name()).collect();
        names.sort_unstable();
        assert_eq!(
            names,
            ["case-sensitive", "preserve-case", "regex", "whole-word"],
        );
        assert_eq!(LEGEND, "Aa");
        assert_eq!(WEIGHT, FontWeight::SEMIBOLD);

        let fixture = SearchToggleIcon::fixture();
        assert_eq!(fixture.toggle, Toggle::PreserveCase);
        assert_eq!(fixture.breakpoint, Breakpoint::Sm);
        assert!(!fixture.active);
        assert!(!fixture.empty);
    }
}
