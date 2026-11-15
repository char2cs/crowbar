use gpui::{App, IntoElement, Pixels, RenderOnce, Styled, Window, px, svg};

use crate::assets::WORDMARK_PATH;
use crate::surface::Surface;
use crate::theme::Theme;

const ASPECT_WIDTH_OVER_HEIGHT: f32 = 453.0 / 115.0;

/// The Crowbar logo drawn in the muted foreground colour of the window surface.
#[derive(Debug, IntoElement)]
pub struct Wordmark {
    width: Pixels,
}

impl Wordmark {
    /// A logo `width` wide; its height follows the artwork's proportions.
    #[must_use]
    pub fn new(width: Pixels) -> Self {
        Self { width }
    }
}

impl RenderOnce for Wordmark {
    fn render(
        self,
        _window: &mut Window,
        cx: &mut App,
    ) -> impl IntoElement {
        let color = Theme::of(cx).palette(Surface::Window).muted_foreground();
        svg()
            .path(WORDMARK_PATH)
            .w(self.width)
            .h(px(f32::from(self.width) / ASPECT_WIDTH_OVER_HEIGHT))
            .flex_none()
            .text_color(color)
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn the_logo_keeps_the_artwork_proportions() {
        let logo = Wordmark::new(px(453.0));
        assert!((f32::from(logo.width) / ASPECT_WIDTH_OVER_HEIGHT - 115.0).abs() < 0.01);
    }
}
