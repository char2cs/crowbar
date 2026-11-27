use gpui::{App, Hsla, IntoElement, Pixels, RenderOnce, Styled, Window, px, svg};

use crate::foundation::icon::assets::WORDMARK_PATH;
use crate::foundation::theme::Surface;
use crate::foundation::theme::Theme;

const ASPECT_WIDTH_OVER_HEIGHT: f32 = 453.0 / 115.0;

#[derive(Debug, IntoElement)]
pub struct Wordmark {
    width: Pixels,
    color: Option<Hsla>,
}

impl Wordmark {
    #[must_use]
    pub fn new(width: Pixels) -> Self {
        Self { width, color: None }
    }

    #[must_use]
    pub fn color(
        mut self,
        color: Hsla,
    ) -> Self {
        self.color = Some(color);
        self
    }
}

impl RenderOnce for Wordmark {
    fn render(
        self,
        _window: &mut Window,
        cx: &mut App,
    ) -> impl IntoElement {
        let muted = Theme::of(cx).palette(Surface::Window).muted_foreground();
        let color = self.color.unwrap_or(muted);
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
