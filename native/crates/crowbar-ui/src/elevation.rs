//! The "raised surface" shadow stack, shared by every control that uses it.
//!
//! # Why this is a module and not two literals at two call sites
//!
//! `web/src/components/layout/workspace-row-base.ts`'s `ROW_ACTIVE` and the
//! tabs primitive's own indicator carry the **same** three-layer treatment, and
//! the live app confirms it: both compute to
//!
//! ```text
//! box-shadow: oklab(1 0 0 / 0.16) 0px 1px 0px 0px inset,
//!             oklab(0 0 0 / 0.1)  0px 1px 2px 0px;
//! ```
//!
//! Two call sites spelling that separately is how the tab glyph's weight
//! drifted from the tab bar's, one item ago. It is written once here.
//!
//! # Why it was missing entirely until now
//!
//! `row_base::active`'s own doc comment used to say the shadow layers "are
//! invisible (`ANCHORS.md` §6)". They are invisible **to the anchor
//! contract** — which records `bounds`, `bg`, `radius`, `border` and text, and
//! has no shadow field at all — and perfectly visible to anyone looking at the
//! window. That is the same reasoning that shipped 23 empty boxes where the
//! app draws icons, and it produced the same class of defect: the active row
//! and the active tab rendered as flat rectangles where the reference draws a
//! lit, raised surface.
//!
//! Measured off the real window: one logical pixel inside the row's top edge,
//! the reference paints `rgb(67, 67, 66)` over the row's own
//! `rgb(31, 31, 30)` — which is white at 16%, exactly what the class declares.
//!
//! # gpui insets from the **border** box; CSS insets from the **padding** box
//!
//! This is why the highlight rendered as nothing at first, with every other
//! link in the chain correct: the colour arithmetic, the unconditional
//! `paint_inset_shadows` call, the Metal shader's zero-blur fast path, and the
//! scene's `(order, kind)` sort all checked out, and an isolation probe
//! (`screenshot.rs`'s `an_inset_shadow_paints_a_hairline_inside_the_top_edge`)
//! proved gpui paints inset shadows exactly as asked.
//!
//! The catch is *where*. gpui's `Window::paint_inset_shadows` measures the
//! hole from the element's **border box**, so `inset 0 1px` covers the top
//! logical pixel of the border box — and gpui paints the border **after** the
//! inset shadows, so on any bordered element the highlight is painted and then
//! immediately covered. Every row here has a one-pixel border (in the same
//! colour as its background, which is why it is otherwise invisible).
//!
//! CSS measures the same shadow from the padding box, i.e. already inside the
//! border. The two agree once the offset absorbs the border width, which is
//! what [`raised`] takes it for — and that also puts the covered part of the
//! highlight exactly under the border, where CSS puts nothing and gpui paints
//! something invisible.

use gpui::{BoxShadow, Pixels, px};

use crate::theme::Color;

/// The inset highlight's opacity, as a percentage —
/// `inset-shadow-[0_1px_--theme(--color-white/16%)]`.
const HIGHLIGHT_PERCENT: f32 = 16.0;

/// The drop shadow's opacity — `shadow-black/10`.
const DROP_PERCENT: f32 = 10.0;

/// The pressed state's inset opacity — `active:inset-shadow-[0_1px_…black/8%]`.
const PRESSED_PERCENT: f32 = 8.0;

/// Tailwind `shadow-xs` is `0 1px 2px 0`; only its colour is overridden.
const DROP_BLUR: f32 = 2.0;

/// One logical pixel down, which is every layer's `y` offset here.
const OFFSET_Y: f32 = 1.0;

/// A raised, inset-lit surface: the resting look of `ROW_ACTIVE` and of the
/// tabs indicator.
///
/// `border_width` is the element's own border, which the highlight's offset
/// has to absorb — see the module docs. Pass `Pixels::ZERO` for an unbordered
/// element.
///
/// Order matches the CSS declaration — inset highlight first, drop shadow
/// second. They do not overlap (one paints inside the border box, one
/// outside), so the order is documentation rather than a compositing
/// requirement.
#[must_use]
pub fn raised(border_width: Pixels) -> Vec<BoxShadow> {
    vec![
        BoxShadow::new(
            px(0.0),
            px(OFFSET_Y) + border_width,
            Color::WHITE
                .mix(HIGHLIGHT_PERCENT, Color::TRANSPARENT)
                .value(),
        )
        .inset(),
        BoxShadow::new(
            px(0.0),
            px(OFFSET_Y),
            Color::BLACK.mix(DROP_PERCENT, Color::TRANSPARENT).value(),
        )
        .blur_radius(px(DROP_BLUR)),
    ]
}

/// The same surface while held down: `active:inset-shadow-[…black/8%]
/// active:shadow-none` — the highlight inverts to a dark inset and the drop
/// shadow is dropped entirely, so the surface reads as pushed in.
#[must_use]
pub fn pressed(border_width: Pixels) -> Vec<BoxShadow> {
    vec![
        BoxShadow::new(
            px(0.0),
            px(OFFSET_Y) + border_width,
            Color::BLACK
                .mix(PRESSED_PERCENT, Color::TRANSPARENT)
                .value(),
        )
        .inset(),
    ]
}

#[cfg(test)]
mod tests {
    use super::{DROP_BLUR, OFFSET_Y, pressed, raised};
    use gpui::px;

    /// The resting stack is the CSS declaration, layer for layer.
    #[test]
    fn raised_is_an_inset_highlight_over_a_drop_shadow() {
        let shadows = raised(px(1.0));
        assert_eq!(shadows.len(), 2);

        let highlight = &shadows[0];
        assert!(
            highlight.inset,
            "the highlight paints inside the border box"
        );
        assert_eq!(
            highlight.offset.y,
            px(OFFSET_Y) + px(1.0),
            "the offset absorbs the border width — see the module docs"
        );
        assert_eq!(highlight.blur_radius, px(0.0), "a hairline, not a glow");
        assert!(
            (highlight.color.a - 0.16).abs() < 0.005,
            "white at 16%, got a={}",
            highlight.color.a
        );
        assert!((highlight.color.l - 1.0).abs() < f32::EPSILON, "white");

        let drop = &shadows[1];
        assert!(!drop.inset, "the drop shadow paints outside");
        assert_eq!(drop.blur_radius, px(DROP_BLUR));
        assert!((drop.color.a - 0.10).abs() < 0.005, "black at 10%");
        assert!(drop.color.l.abs() < f32::EPSILON, "black");
    }

    /// Pressed drops the drop shadow — `active:shadow-none` — and inverts the
    /// inset. Both halves asserted: keeping the highlight *and* adding a dark
    /// inset would still be two layers and would still look wrong.
    #[test]
    fn pressed_inverts_the_inset_and_drops_the_shadow() {
        let shadows = pressed(px(1.0));
        assert_eq!(shadows.len(), 1, "active:shadow-none removes the drop");
        assert!(shadows[0].inset);
        assert!(shadows[0].color.l.abs() < f32::EPSILON, "black, not white");
        assert!((shadows[0].color.a - 0.08).abs() < 0.005);
    }
}
