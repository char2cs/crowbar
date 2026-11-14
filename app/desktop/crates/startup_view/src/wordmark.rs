use gpui::{App, IntoElement, ParentElement, RenderOnce, Styled, Window, div, px};
use ui::{Surface, Theme, font};

const SIZE: f32 = 44.0;

#[derive(Debug, IntoElement)]
pub(crate) struct Wordmark;

impl RenderOnce for Wordmark {
    fn render(self, _window: &mut Window, cx: &mut App) -> impl IntoElement {
        let palette = Theme::of(cx).palette(Surface::Window);
        div()
            .font_family(font::HEADING_FAMILY)
            .text_size(px(SIZE))
            .line_height(px(SIZE * 1.1))
            .text_color(palette.foreground())
            .child("Crowbar")
    }
}

#[cfg(test)]
#[path = "wordmark_tests.rs"]
mod tests;
