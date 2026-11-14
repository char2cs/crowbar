//! One pixel divider lines.

use gpui::{App, Div, IntoElement, Pixels, RenderOnce, Styled, Window, div, px};

use crate::axis::Axis;
use crate::surface::Surface;
use crate::theme::Theme;

/// A 1px line in the border colour.
#[derive(Debug, Clone, Copy, IntoElement)]
pub struct Separator {
    axis: Axis,
    surface: Surface,
}

impl Separator {
    /// A divider that runs along the horizontal axis (a rule between stacked items).
    pub fn horizontal() -> Self {
        Self { axis: Axis::Horizontal, surface: Surface::default() }
    }

    /// A divider that runs along the vertical axis (a rule between side-by-side items).
    pub fn vertical() -> Self {
        Self { axis: Axis::Vertical, surface: Surface::default() }
    }

    /// Paints with the palette of `surface`.
    #[must_use]
    pub fn on(mut self, surface: Surface) -> Self {
        self.surface = surface;
        self
    }
}

const THICKNESS: Pixels = px(1.0);

impl RenderOnce for Separator {
    fn render(self, _window: &mut Window, cx: &mut App) -> impl IntoElement {
        let color = Theme::of(cx).palette(self.surface).border();
        let line: Div = div().flex_none().bg(color);
        match self.axis {
            Axis::Horizontal => line.h(THICKNESS).w_full(),
            Axis::Vertical => line.w(THICKNESS).h_full(),
        }
    }
}

#[cfg(test)]
#[path = "separator_tests.rs"]
mod tests;
