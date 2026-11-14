//! The draggable divider between two resizable regions.

use gpui::{
    App, CursorStyle, ElementId, InteractiveElement, IntoElement, Pixels, RenderOnce, Styled, Window, div, px,
};

use crate::axis::Axis;
use crate::hover;
use crate::sash_drag::SashDrag;
use crate::surface::Surface;
use crate::theme::Theme;

const HIT_AREA: f32 = 6.0;
const HOVER_FILL: f32 = 0.6;

/// A 6px hit area that tints with `border/60` on hover; the owner reads [`SashDrag`] moves.
#[derive(IntoElement)]
pub struct Sash {
    id: ElementId,
    axis: Axis,
    surface: Surface,
}

impl Sash {
    /// A sash that separates regions along `axis`: `Horizontal` for a vertical bar between side-by-side regions.
    pub fn new(id: impl Into<ElementId>, axis: Axis) -> Self {
        Self { id: id.into(), axis, surface: Surface::default() }
    }

    /// Paints with the palette of `surface`.
    #[must_use]
    pub fn on(mut self, surface: Surface) -> Self {
        self.surface = surface;
        self
    }

    /// Width of the pointer hit area.
    pub fn thickness() -> Pixels {
        px(HIT_AREA)
    }
}

impl RenderOnce for Sash {
    fn render(self, window: &mut Window, cx: &mut App) -> impl IntoElement {
        let palette = Theme::of(cx).palette(self.surface);
        let state = hover::track(&self.id, "hover", false, window, cx);
        let rest = palette.border().opacity(0.0);
        let color = hover::color(&state, rest, palette.border().opacity(HOVER_FILL), window, cx);
        let axis = self.axis;
        let bar = div().id(self.id).flex_none().bg(color).on_hover(hover::listener(&state));
        let bar = match axis {
            Axis::Horizontal => bar.w(Self::thickness()).h_full().cursor(CursorStyle::ResizeLeftRight),
            Axis::Vertical => bar.h(Self::thickness()).w_full().cursor(CursorStyle::ResizeUpDown),
        };
        bar.on_drag(SashDrag { axis }, |drag, _, _, cx| cx.new(|_| drag.clone()))
    }
}

#[cfg(test)]
#[path = "sash_tests.rs"]
mod tests;
