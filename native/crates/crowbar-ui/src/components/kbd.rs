//! `kbd` — a keycap, and `badge`'s "authored height is not line-sized" rule
//! confirmed on a second component.
//!
//! The native half of `web/src/components/ui/kbd.tsx`, which exports two things:
//! `Kbd`, a keycap, and `KbdGroup`, a bare `inline-flex` that holds several.
//! Every value below came out of the app's own `tailwindcss` 4.3.0 with the
//! utility as a candidate — the method `native/MAPPING.md` fixes — and each is
//! confirmed against the captured reference. See `native/mapping/kbd.md`.
//!
//! # `border.w` is **0** here, which is the mirror of `badge`'s trap
//!
//! `native/MAPPING.md` records that `border` is 1px on every button and badge
//! variant, because those class lists carry a bare `border`. **`kbd.tsx` does
//! not.** Preflight sets `border: 0 solid` on every element and nothing puts it
//! back, so the keycap pays no border pixel at all. Measured live:
//! `borderTopWidth: "0px"`, and the reference agrees — `border.w 0` with a
//! colour of `#ffffff0f` that is never painted.
//!
//! This is why the trap is stated in `MAPPING.md` as "measure rather than
//! infer" in *both* directions: a port that carried `badge`'s pixel across
//! would have made every keycap 2px wider and 2px shorter in its content box.
//!
//! # Not `line_sized` — `badge`'s precedent, a second time
//!
//! `h-5` authors the box at 20px around a 16px line box. `ANCHORS.md` v1.6 is
//! explicit that the test is whether the height is *derived from* the line box,
//! not whether the element paints text; declaring it here would compare 20
//! against 16 and manufacture a **4px delta on the surface's only anchor**. The
//! reference is the evidence: `bounds.h 20`, `font.line_height 16`.
//!
//! # `KbdGroup` is ported but **not anchored**, and that is `ANCHORS.md` v1.8
//!
//! The group is real — the workspace switcher's footer wraps its ↑/↓ caps in
//! one — but it cannot be a snapshot root. `data-oracle-id` lives on the
//! *primitive*, so every keycap inside a group carries the id `kbd`, and a
//! snapshot rooted at the group would contain that id **twice**. v1.8 ranks
//! that a refusal rather than a delta, and is right to: the differ matches by
//! id and would have no way to say which of the two it compared.
//!
//! So [`KbdGroup`] exists here as the layout its children sit in — it is what
//! makes a two-cap row 44px wide rather than 40 — and the surface renders it as
//! an ancestor without an id of its own. Its one measurable oddity — it does not
//! set `font-sans`, so it keeps the UA's monospace default — is recorded on
//! [`KbdGroup`] itself.

use gpui::{
    AnyElement, Div, FontWeight, IntoElement as _, ParentElement as _, Pixels, Rems, SharedString,
    Styled as _, div, px, relative,
};

use super::anchor::{AnchorId, AnchorSink};
use super::badge::TypeStep;
use crate::theme::{Theme, ui_sans_font};

/// The keycap anchor — the only one this surface carries.
pub const ID_KBD: &str = "kbd";

/// A keycap's used width is its label's max-content width plus `px-1`, floored
/// by `min-w-5` — and `min-w-*` is a floor, never a stretch, so the box is
/// content-sized exactly as `badge`'s is.
pub const CONTENT_SIZED: [&str; 1] = [ID_KBD];

/// **Nothing.** `h-5` authors the height — see the module docs.
pub const LINE_SIZED: [&str; 0] = [];

/// `h-5` — `calc(var(--spacing) * 5)`, measured live as `height: 20px`.
pub const HEIGHT: Pixels = px(20.0);

/// `min-w-5` — the same 20px, as a floor. Three of the four live keycaps hold
/// icons and land exactly on it; the `Esc` cap overruns it, which is what makes
/// the reference cell the interesting one.
pub const MIN_WIDTH: Pixels = px(20.0);

/// `px-1` — `calc(var(--spacing) * 1)`, measured live as `padding-inline: 4px`.
pub const PADDING_X: Pixels = px(4.0);

/// `gap-1` — 4px, shared by the keycap and by [`KbdGroup`].
pub const GAP: Pixels = px(4.0);

/// `rounded` — a **literal `0.25rem`** in the compiled CSS, not one of the
/// theme's `--radius-*` steps.
///
/// Worth pinning: `rounded-sm` next door is `calc(var(--radius) * 0.6)` = 6px
/// and moves with the theme, where this does not. Measured live as
/// `border-radius: 4px`, and the reference says `radius 4`.
pub const RADIUS: Pixels = px(4.0);

/// `[&_svg:not([class*='size-'])]:size-3` — 12px, the extent of an icon a call
/// site drops into a cap without sizing it itself.
pub const GLYPH: Pixels = px(12.0);

/// `font-medium`.
pub const WEIGHT: FontWeight = FontWeight::MEDIUM;

/// `text-xs` — 12px on Tailwind's stock `--text-xs--line-height` of `1/0.75`,
/// which resolves to the reference's `line_height` of exactly 16.
pub const TYPE_STEP: TypeStep = TypeStep {
    size: Rems(0.75),
    line_height: 16.0 / 12.0,
};

/// What a keycap paints.
///
/// The live footer has both shapes and they measure differently, which is why
/// this is a vocabulary and not a `SharedString`: an icon cap sits on
/// [`MIN_WIDTH`] and a text cap overruns it.
#[derive(Clone, Debug, PartialEq)]
pub enum Cap {
    /// A `<svg>` a call site supplies. Drawn as an empty [`GLYPH`] box, the
    /// same call every component since `git_status_row` has made about icons:
    /// there is no native equivalent and drawing a substitute would put a shape
    /// on screen for the oracle to converge on.
    Glyph,
    /// A string. **The captured cell is `Esc`.**
    Text(SharedString),
    /// No child at all.
    ///
    /// `<Kbd />` takes its content from the call site, so a cap with neither a
    /// glyph nor a legend is an expressible — if unused — rendering, and it is
    /// the one picture where the box is `min-w-5` exactly and the anchor
    /// carries no text half. §8.3's `empty` is that cell.
    Empty,
}

/// One `<Kbd>`.
#[derive(Clone, Debug, PartialEq)]
pub struct Kbd {
    /// What the cap paints.
    pub cap: Cap,
}

impl Kbd {
    /// The captured cell: the workspace switcher's `Esc` cap at a 1714px
    /// viewport — `27.61 × 20`, `bg #ffffff0a`, `fg #a4a4a4ff`, `radius 4`,
    /// `border.w 0`, `CalSansUI` 12/16 at weight 500.
    #[must_use]
    pub fn fixture() -> Self {
        Self {
            cap: Cap::Text(SharedString::new_static("Esc")),
        }
    }

    /// The keycap's own box.
    ///
    /// `inline-flex` becomes `.flex()` for `badge`'s reason: gpui has no inline
    /// flow, and the live computed `display` is **`flex`** anyway, because every
    /// keycap is a flex item and CSS blockifies those.
    ///
    /// `font-sans` is named rather than inherited, and on this component that is
    /// load-bearing rather than hygienic: a bare `<kbd>` inherits the UA's
    /// monospace default, which is what [`KbdGroup`] actually renders. Measured
    /// live — the cap says `CalSansUI`, the group says
    /// `JetBrains Mono Variable`. [`ui_sans_font`] carries the fallback chain a
    /// keycap needs for exactly this component's most common legend — a macOS
    /// modifier symbol, which `CalSansUI`'s own cmap does not cover (P3.24).
    ///
    /// `pointer-events-none` and `select-none` are not visual properties and
    /// land nowhere.
    fn shell(theme: &Theme) -> Div {
        div()
            .font(ui_sans_font(theme))
            .flex()
            .items_center()
            .justify_center()
            .gap(GAP)
            .h(HEIGHT)
            .min_w(MIN_WIDTH)
            .px(PADDING_X)
            .rounded(RADIUS)
            .bg(theme.color_muted)
            .text_size(TYPE_STEP.size)
            .line_height(relative(TYPE_STEP.line_height))
            .font_weight(WEIGHT)
            .text_color(theme.color_muted_foreground)
    }

    /// A call site's `<svg>`, as an empty box.
    fn glyph_box() -> Div {
        div().flex_shrink_0().w(GLYPH).h(GLYPH)
    }

    /// The element, with its one anchor.
    pub fn render(&self, theme: &Theme, anchors: &dyn AnchorSink) -> AnyElement {
        let id = AnchorId::from(ID_KBD).content_sized();
        let shell = Self::shell(theme);
        match &self.cap {
            Cap::Text(text) => anchors.boxed_text(id, shell, text.clone()),
            Cap::Glyph => anchors.boxed(id, shell.child(Self::glyph_box())),
            Cap::Empty => anchors.boxed(id, shell),
        }
    }
}

/// `<KbdGroup>` — `inline-flex items-center gap-1`, and nothing else.
///
/// Not anchored; see the module docs for the `ANCHORS.md` v1.8 reason.
///
/// **The group does not set `font-sans`, so it keeps the UA's monospace default
/// for `<kbd>`** — measured live as `JetBrains Mono Variable` against the cap's
/// `CalSansUI`. It paints no text of its own, so nothing observable follows and
/// no anchor records it; it is written down because "two elements in one file
/// resolve different families" is the kind of difference that stays invisible
/// until something inherits through it.
#[derive(Clone, Debug, PartialEq)]
pub struct KbdGroup {
    /// The caps in the group. The live one holds two.
    pub caps: Vec<Kbd>,
}

impl KbdGroup {
    /// The live group: the workspace switcher's ↑/↓ pair, measured at `44 × 20`
    /// — two [`MIN_WIDTH`] caps and one [`GAP`].
    #[must_use]
    pub fn fixture() -> Self {
        Self {
            caps: vec![Kbd { cap: Cap::Glyph }, Kbd { cap: Cap::Glyph }],
        }
    }

    /// The group's box. No background, no radius, no border — it is a layout
    /// and nothing else.
    fn shell() -> Div {
        div().flex().items_center().gap(GAP)
    }

    /// The group and its caps. The caps carry the anchors; the group does not.
    pub fn render(&self, theme: &Theme, anchors: &dyn AnchorSink) -> AnyElement {
        let mut element = Self::shell();
        for cap in &self.caps {
            element = element.child(cap.render(theme, anchors));
        }
        element.into_any_element()
    }
}

#[cfg(test)]
mod tests {
    use super::{
        Cap, GAP, HEIGHT, ID_KBD, Kbd, KbdGroup, MIN_WIDTH, PADDING_X, RADIUS, TYPE_STEP, WEIGHT,
    };
    use gpui::{FontWeight, Rems, px};

    /// The cap is content-sized and **not** line-sized — the `badge` precedent,
    /// which is the single most consequential declaration on this component.
    #[test]
    fn the_cap_is_content_sized_and_never_line_sized() {
        assert_eq!(super::CONTENT_SIZED, [ID_KBD]);
        assert!(
            super::LINE_SIZED.is_empty(),
            "h-5 authors the height; declaring it would compare 20 against 16",
        );
    }

    /// The authored height and the line box are **different numbers**, which is
    /// what makes the declaration above load-bearing rather than cosmetic. A
    /// control: if these were ever equal, the test above would pass either way
    /// and stop meaning anything.
    #[test]
    fn the_authored_height_really_does_differ_from_the_line_box() {
        let line_box = TYPE_STEP.size.0 * 16.0 * TYPE_STEP.line_height;
        assert!(
            (line_box - 16.0).abs() < 1e-4,
            "line box is 16, got {line_box}"
        );
        assert_eq!(HEIGHT, px(20.0));
        assert!(
            (f32::from(HEIGHT) - line_box).abs() > 0.5,
            "a 4px gap is exactly the delta v1.6 warns a wrong declaration invents",
        );
    }

    /// The reference's box, reproduced from the constants: `min-w-5` floors an
    /// icon cap at 20, and `px-1` adds 8 to a text cap's advance width. The
    /// captured `Esc` cap is `27.61` against a `text_width` of `19.61`.
    #[test]
    fn the_padding_accounts_for_the_captured_width() {
        assert_eq!(PADDING_X, px(4.0));
        let advance = 19.61_f32;
        assert!(
            ((advance + 2.0 * f32::from(PADDING_X)) - 27.61).abs() < 0.01,
            "19.61 + 2×4 should be the reference's 27.61",
        );
        // And the floor does not bind on that cell, which is why it is the
        // interesting one.
        assert!(advance + 2.0 * f32::from(PADDING_X) > f32::from(MIN_WIDTH));
    }

    /// `rounded` is a literal 4px, not a theme radius step — see [`RADIUS`].
    #[test]
    fn the_radius_is_the_literal_quarter_rem() {
        assert_eq!(RADIUS, px(4.0));
    }

    /// `text-xs` at weight 500, which is what the reference reports.
    #[test]
    fn the_type_step_is_the_references() {
        assert_eq!(TYPE_STEP.size, Rems(0.75));
        assert!((TYPE_STEP.size.0 * 16.0 - 12.0).abs() < f32::EPSILON);
        assert_eq!(WEIGHT, FontWeight::MEDIUM);
    }

    /// The live group is two icon caps and one gap: `20 + 4 + 20 = 44`, which is
    /// what the live group measured.
    #[test]
    fn the_group_fixture_is_the_measured_forty_four_pixels() {
        let group = KbdGroup::fixture();
        assert_eq!(group.caps.len(), 2);
        for cap in &group.caps {
            assert_eq!(cap.cap, Cap::Glyph);
        }
        let width = 2.0 * f32::from(MIN_WIDTH) + f32::from(GAP);
        assert!((width - 44.0).abs() < f32::EPSILON, "got {width}");
    }

    /// The fixture is the captured `Esc` cap.
    #[test]
    fn the_fixture_is_the_captured_esc_cap() {
        assert_eq!(
            Kbd::fixture().cap,
            Cap::Text(gpui::SharedString::new_static("Esc")),
        );
    }
}
